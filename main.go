package main

import ( //Necessary imports for the window, the widgets, and the background loading
	"context"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type explorer struct { //Struct that holds everything the window needs to remember: the data, the key, and every widget that gets updated
	apiKey    string
	countries []Country //The countries of the region currently chosen, which is what the list displays
	requestID int       //Counts region requests so an old, slow response can be recognized and ignored
	flagID    int       //Same idea for flag downloads

	status       *widget.Label //Line of text under the drop-down that reports loading, errors, and how many countries were found
	list         *widget.List
	heading      *widget.Label
	flagImage    *canvas.Image
	flagNote     *widget.Label
	officialName *widget.Label
	commonNames  *widget.Label
	languages    *widget.Label
	landlocked   *widget.Label
	capitals     *widget.Label
	population   *widget.Label
}

// Function used to make a label that wraps onto several lines instead of running off the edge of the window
func newWrapLabel() *widget.Label {
	label := widget.NewLabel("")
	label.Wrapping = fyne.TextWrapWord
	return label
}

// Method that creates every widget and arranges them, returning the finished layout and the region drop-down
func (e *explorer) build() (fyne.CanvasObject, *widget.Select) {
	e.status = newWrapLabel()
	e.status.SetText("Choose a region to load its countries.")

	//The list is given three functions: how many items there are, what an empty row looks like, and how to fill a row with a country name
	e.list = widget.NewList(
		func() int { return len(e.countries) },
		func() fyne.CanvasObject { return widget.NewLabel("A fairly long country name") },
		func(id widget.ListItemID, item fyne.CanvasObject) {
			if id < len(e.countries) { //Boolean condition protecting against a row that no longer exists after the list changes
				item.(*widget.Label).SetText(e.countries[id].Names.Common)
			}
		},
	)
	e.list.OnSelected = func(id widget.ListItemID) { //Runs every time the user clicks a country in the list
		if id < len(e.countries) {
			e.showCountry(e.countries[id])
		}
	}

	regions := widget.NewSelect([]string{"Africa", "Americas", "Asia", "Europe", "Oceania"}, e.loadRegion) //Drop-down that calls loadRegion whenever a region is picked
	regions.PlaceHolder = "Choose a region..."

	left := container.NewBorder( //Left side: the drop-down and status on top, with the list filling the rest of the space
		container.NewVBox(widget.NewLabelWithStyle("Region", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), regions, e.status),
		nil, nil, nil,
		e.list,
	)

	e.heading = widget.NewLabelWithStyle("Select a country from the list", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	e.flagImage = canvas.NewImageFromResource(nil)
	e.flagImage.FillMode = canvas.ImageFillContain //Keeps the flag's proportions instead of stretching it
	e.flagImage.SetMinSize(fyne.NewSize(320, 200))
	e.flagNote = newWrapLabel()
	e.officialName = newWrapLabel()
	e.commonNames = newWrapLabel()
	e.languages = newWrapLabel()
	e.landlocked = newWrapLabel()
	e.capitals = newWrapLabel()
	e.population = newWrapLabel()

	form := widget.NewForm( //Form pairs each piece of data with a label so the user knows what they are looking at
		widget.NewFormItem("Official name", e.officialName),
		widget.NewFormItem("Common name(s)", e.commonNames),
		widget.NewFormItem("Official language(s)", e.languages),
		widget.NewFormItem("Flag", container.NewVBox(e.flagImage, e.flagNote)),
		widget.NewFormItem("Landlocked?", e.landlocked),
		widget.NewFormItem("Capital(s)", e.capitals),
		widget.NewFormItem("Population", e.population),
	)
	right := container.NewVScroll(container.NewVBox(e.heading, widget.NewSeparator(), form)) //Right side scrolls so long text is never cut off

	split := container.NewHSplit(left, right)
	split.Offset = 0.3 //The list takes up roughly 30 percent of the window width
	return split, regions
}

// Method that loads the countries of the chosen region without freezing the window
func (e *explorer) loadRegion(region string) {
	e.requestID++
	id := e.requestID //Remembers which request this is so a newer click can cancel its effect
	e.countries = nil
	e.list.UnselectAll()
	e.list.Refresh()
	e.clearDetails()
	e.status.SetText("Loading countries in " + region + "...")

	go func() { //Goroutine so the slow network request runs in the background
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		countries, err := fetchRegion(ctx, e.apiKey, region)

		fyne.Do(func() { //Window changes must be handed back to the main thread, and fyne.Do does that
			if id != e.requestID { //Boolean condition checking if the user picked a different region while this one was loading
				return
			}
			if err != nil {
				e.status.SetText("Could not load " + region + ": " + err.Error())
				return
			}
			e.countries = countries
			e.list.Refresh()
			e.status.SetText(fmt.Sprintf("%d countries in %s. Select one to see its details.", len(countries), region))
		})
	}()
}

// Method that blanks out the details panel, used when the region changes
func (e *explorer) clearDetails() {
	e.flagID++ //Also cancels any flag download that is still in progress
	e.heading.SetText("Select a country from the list")
	for _, label := range []*widget.Label{e.officialName, e.commonNames, e.languages, e.landlocked, e.capitals, e.population, e.flagNote} {
		label.SetText("")
	}
	e.flagImage.Resource = nil
	e.flagImage.Refresh()
}

// Function used to show "Not available" instead of leaving a blank when a piece of data is missing
func orNotAvailable(s string) string {
	if strings.TrimSpace(s) == "" {
		return "Not available"
	}
	return s
}

// Method that fills in every labeled field for the country the user clicked
func (e *explorer) showCountry(c Country) {
	e.heading.SetText(c.Names.Common)
	e.officialName.SetText(orNotAvailable(c.Names.Official))
	e.commonNames.SetText(orNotAvailable(c.commonNames()))
	e.languages.SetText(orNotAvailable(strings.Join(c.languageNames(), ", ")))
	if c.Landlocked { //Turns the true/false value into a sentence the user can understand
		e.landlocked.SetText("Yes, this country has no coastline.")
	} else {
		e.landlocked.SetText("No, this country has a coastline.")
	}
	e.capitals.SetText(orNotAvailable(c.capitalNames()))
	e.population.SetText(populationText(c.Population))
	e.loadFlag(c)
}

// Method that downloads the flag in the background and shows it once it arrives
func (e *explorer) loadFlag(c Country) {
	e.flagID++
	id := e.flagID //Same trick as loadRegion, so a slow flag from a previous click never replaces the current one
	e.flagImage.Resource = nil
	e.flagImage.Refresh()
	e.flagNote.SetText("Loading flag...")
	flagURL, name := c.Flag.URLPNG, c.Names.Common

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		data, err := fetchFlag(ctx, flagURL)

		fyne.Do(func() {
			if id != e.flagID {
				return
			}
			if err != nil {
				e.flagNote.SetText("Flag unavailable: " + err.Error())
				return
			}
			e.flagImage.Resource = fyne.NewStaticResource(name+".png", data) //Wraps the downloaded bytes so the image widget can display them
			e.flagImage.Refresh()
			e.flagNote.SetText("")
		})
	}()
}

// func main operates as the entry point of the program, creating the window and starting it
func main() {
	a := app.New()
	w := a.NewWindow("Bertin Project 3 - Country Explorer")

	e := &explorer{}
	content, regions := e.build()
	w.SetContent(content)
	w.Resize(fyne.NewSize(1000, 650)) //Fits comfortably inside the 1920x1080 monitor the assignment mentions

	key, err := loadAPIKey()
	if err != nil { //Without a key nothing can be loaded, so the drop-down is disabled and the status line explains why
		e.status.SetText("Setup needed: " + err.Error())
		regions.Disable()
	}
	e.apiKey = key

	w.ShowAndRun() //Opens the window and keeps the program running until it is closed
}
