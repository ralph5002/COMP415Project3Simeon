package main

import ( //Necesarry imports needed for the project to run properly
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Below the parameters for our mini webpage we're creating using the program
var baseURL = "https://api.restcountries.com/countries/v5" //Web address of the REST Countries API that every request starts from

const pageSize = 100 //Most countries the free plan lets us ask for at once, so bigger regions are loaded in pages of 100

var httpClient = &http.Client{Timeout: 30 * time.Second} //Our own client with a timeout, since the default client would wait forever if the internet is down

type Names struct { //Struct designated as "names" to bear certain characteristics based on what is common, official, and alternative
	Common     string   `json:"common"` //The tags in backticks tell the JSON decoder which key in the response fills each field
	Official   string   `json:"official"`
	Alternates []string `json:"alternates"`
}

type Capital struct { //Struct designated as "Capital" to indicate the capital of the relevant nation
	Name string `json:"name"`
}

type Flag struct { //Flag struct consisting of an emoji label and image
	Emoji  string `json:"emoji"`
	URLPNG string `json:"url_png"` //Web address of the flag picture that gets downloaded and displayed
}

type Country struct { //Country struct consists of necessary attributes (names, Captials, Region, Subregion, etc.)
	Names        Names           `json:"names"`
	Capitals     []Capital       `json:"capitals"` //A slice because some countries have more than one capital
	Region       string          `json:"region"`
	Subregion    string          `json:"subregion"`
	Landlocked   bool            `json:"landlocked"`
	Population   float64         `json:"population"`
	Flag         Flag            `json:"flag"`
	LanguagesRaw json.RawMessage `json:"languages"` //Kept as raw JSON and decoded later by languageNames, since the exact shape of this field was not known ahead of time
}

type apiResponse struct { //Struct within a struct
	Data struct {
		Objects []Country `json:"objects"` //The list of countries returned by the API
		Meta    struct {
			Total int  `json:"total"`
			More  bool `json:"more"` //True when there are more pages of countries still to request
		} `json:"meta"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"` //Filled in by the API when something goes wrong, such as a bad key
	} `json:"errors"`
}

func loadAPIKey() (string, error) { //load API function checks that key exists, scans it properly, and handles in the event of a missing key
	//First place to look is an environment variable so the key never has to be written inside the code
	if key := strings.TrimSpace(os.Getenv("RESTCOUNTRIES_API_KEY")); key != "" {
		return key, nil
	}
	//Second place to look is a file named apikey.txt next to the program
	if data, err := os.ReadFile("apikey.txt"); err == nil {
		if key := strings.TrimSpace(string(data)); key != "" {
			return key, nil
		}
	}
	return "", errors.New("no API key found: set the RESTCOUNTRIES_API_KEY environment variable or put the key in a file named apikey.txt next to the program")
}

// Function used to collect every country in a region, requesting one page at a time until the API says there are no more
func fetchRegion(ctx context.Context, apiKey, region string) ([]Country, error) {
	var all []Country
	for offset := 0; ; offset += pageSize { //Offset moves forward by one page each time through the loop
		endpoint := fmt.Sprintf("%s/region/%s?limit=%d&offset=%d&response_fields_omit=names.translations",
			baseURL, url.PathEscape(region), pageSize, offset) //Builds the web address for this page, leaving out the translations to keep the response small
		page, more, err := fetchPage(ctx, apiKey, endpoint)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if !more || len(page) == 0 { //Stops once the API reports no more pages (or sends an empty page)
			break
		}
	}
	return all, nil
}

// Function used to make one request to the API and turn the JSON that comes back into Country structs
func fetchPage(ctx context.Context, apiKey, endpoint string) ([]Country, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey) //The API key is sent in this header so the API knows who is asking
	req.Header.Set("Accept", "application/json")

	res, err := httpClient.Do(req)
	if err != nil { //This error means the request never got an answer, such as no internet or a timeout
		return nil, false, fmt.Errorf("could not reach the API: %w", err)
	}
	defer res.Body.Close() //Ensures the response body is closed after the function is executed

	var parsed apiResponse
	decodeErr := json.NewDecoder(res.Body).Decode(&parsed) //Decoded before the status is checked so an error message from the API can still be read

	if res.StatusCode != http.StatusOK { //A reply that is not 200 is not an error in Go, so the status code has to be checked by hand
		return nil, false, describeStatus(res.StatusCode, parsed)
	}
	if decodeErr != nil {
		return nil, false, fmt.Errorf("could not read the API response: %w", decodeErr)
	}
	return parsed.Data.Objects, parsed.Data.Meta.More, nil
}

// Function used to turn a failing status code into a message the user can understand
func describeStatus(status int, parsed apiResponse) error {
	detail := ""
	if len(parsed.Errors) > 0 {
		detail = ": " + parsed.Errors[0].Message
	}
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("the API key was rejected (401)%s", detail)
	case http.StatusForbidden:
		return fmt.Errorf("access forbidden (403), the account may be over its monthly limit%s", detail)
	case http.StatusTooManyRequests:
		return fmt.Errorf("too many requests (429), wait a few seconds and try again%s", detail)
	}
	return fmt.Errorf("the API returned status %d%s", status, detail)
}

// Function used to download the flag picture so it can be shown in the window
func fetchFlag(ctx context.Context, flagURL string) ([]byte, error) {
	if flagURL == "" { //Boolean condition checking if the country has a flag picture listed at all
		return nil, errors.New("no flag image listed for this country")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, flagURL, nil)
	if err != nil {
		return nil, err
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("flag server returned status %d", res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, 5<<20)) //Reads the picture but caps it at 5 MB so a huge file cannot flood the memory
}

// Method that pulls the language names out of the raw JSON, handling both a list and a keyed object
func (c Country) languageNames() []string {
	var names []string
	add := func(v any) { //Small function inside a function (a closure) that adds one language to names
		if m, ok := v.(map[string]any); ok { //Type assertion checking that this item really is a JSON object
			if label := languageLabel(m); label != "" {
				names = append(names, label)
			}
		}
	}

	var asList []any
	if err := json.Unmarshal(c.LanguagesRaw, &asList); err == nil { //First attempt treats the languages as a list
		for _, item := range asList {
			add(item)
		}
		return names
	}
	var asMap map[string]any
	if err := json.Unmarshal(c.LanguagesRaw, &asMap); err == nil { //Second attempt treats the languages as an object keyed by language code
		for _, item := range asMap {
			add(item)
		}
	}
	return names
}

var languageNameKeys = []string{"name", "english_name", "english", "name_english", "label", "common"} //Possible keys the language name could be stored under, tried in order

// Function used to find the readable name of one language, since the exact field name was not known ahead of time
func languageLabel(m map[string]any) string {
	for _, key := range languageNameKeys {
		switch v := m[key].(type) { //Type switch deciding what to do based on what kind of value sits under the key
		case string:
			if strings.TrimSpace(v) != "" {
				return v
			}
		case map[string]any:
			if label := languageLabel(v); label != "" { //The function calls itself to look inside a nested object
				return label
			}
		}
	}
	best := "" //If none of the expected keys exist, fall back to the longest piece of text, which is most likely the name
	for _, v := range m {
		if s, ok := v.(string); ok && len(s) > len(best) {
			best = s
		}
	}
	return best
}

// Method that combines the main common name with any alternate names, skipping repeats
func (c Country) commonNames() string {
	names := []string{}
	if c.Names.Common != "" {
		names = append(names, c.Names.Common)
	}
	for _, alt := range c.Names.Alternates {
		if alt != "" && alt != c.Names.Common {
			names = append(names, alt)
		}
	}
	return strings.Join(names, ", ")
}

// Method that joins the capital cities into one line of text
func (c Country) capitalNames() string {
	names := make([]string, 0, len(c.Capitals))
	for _, capital := range c.Capitals {
		if capital.Name != "" {
			names = append(names, capital.Name)
		}
	}
	return strings.Join(names, ", ")
}

// Function used to format the population with commas, same idea as the money function in the salary project
func populationText(p float64) string {
	if p <= 0 { //Treats a missing or zero population as not available
		return "Not available"
	}
	digits := strconv.FormatInt(int64(p+0.5), 10) //Rounds to a whole number and converts it to text
	var b strings.Builder
	for i, ch := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 { //A comma goes in before every group of three digits counted from the right
			b.WriteByte(',')
		}
		b.WriteRune(ch)
	}
	return b.String()
}
