// ﷽
// (Bismillahirrahmanirrahim)
// NUSWhereBot Beta
// Author: Faruq Rasid <me@ruqqq.sg>
//
// A Telegram bot which retrieves the map for the queried location.
// When running, make sure to specify TELEGRAM_SECRET env variable
// with the bot's secret key.
//
// Retrieve the required libraries below via go get

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/franela/goreq"
	"github.com/tucnak/telebot"
)

// Structs definitions

// Query string for calling StreetDirectory API (search via keyword)
type SDApiQuery struct {
	Mode    string `url:"mode"`
	Act     string `url:"act"`
	Output  string `url:"output"`
	Limit   int    `url:"limit"`
	Country string `url:"country"`
	Profile string `url:"profile"`
	Q       string `url:"q"`
}

// Query string for calling StreetDirectory API (get png of map given a coord)
type SDMapQuery struct {
	Level int     `url:"level"`
	Lon   float64 `url:"lon"`
	Lat   float64 `url:"lat"`
	SizeX int     `url:"sizex"`
	SizeY int     `url:"sizey"`
	Star  int     `url:"star"`
}

// Struct to store location name, lat, lng
type LocationInfo struct {
	Name string
	Lng  float64
	Lat  float64
}

// Just a request counter
var requestCount int64

// Folder to store the map cache
var MAP_CACHE_DIR = "maps"

func main() {
	bot, err := telebot.NewBot(os.Getenv("TELEGRAM_SECRET"))
	if err != nil {
		panic(err)
	}

	messages := make(chan telebot.Message)
	bot.Listen(messages, 1*time.Second)

	fmt.Println("Bot started.")

	// Message handler
	for message := range messages {
		if message.Text == "/help" || message.Text == "/start" {
			bot.SendMessage(message.Chat, "Hello, "+message.Sender.FirstName+"! To get the location of a place in NUS, just type the keywords of the place you're looking for.", nil)
		} else {
			bot.SendMessage(message.Chat, "Searching Skynet...", nil)
			locations, err := getLocationInfoNUS(message.Text)
			if err != nil {
				bot.SendMessage(message.Chat, "Oops! I encountered an error while searching for the location you requested. Please try again later.", nil)
				bot.SendMessage(message.Chat, err.Error(), nil)
				continue
			}

			if len(locations) == 0 {
				bot.SendMessage(message.Chat, "Oops! I cannot find any result with your search query.", nil)
				continue
			}

			for _, location := range locations {
				bot.SendMessage(message.Chat, "Found! Sending you the map...", nil)
				photo, err := getLocationMap(location)
				if err != nil {
					bot.SendMessage(message.Chat, "Oops! I encountered an error while searching for the location you requested. Please try again later.", nil)
					bot.SendMessage(message.Chat, err.Error(), nil)
					continue
				}

				bot.SendPhoto(message.Chat, photo, nil)
			}
		}

		// Keep track of number of requests (no particular reason to)
		requestCount++
		fmt.Printf("Total Requests: %d\n", requestCount)
	}
}

// Get location (name, lat and lng)
// via NUS Web
func getLocationInfoNUS(query string) ([]LocationInfo, error) {
	url := fmt.Sprintf("http://map.nus.edu.sg/index.php/search/by/%s", query)
	doc, err := goquery.NewDocument(url)
	if err != nil {
		return nil, err
	}

	var locations []LocationInfo

	s := doc.Find("#search_list a[href=\"javascript:void(0)\"]").First()

	onclick, _ := s.Attr("onclick")
	regex := regexp.MustCompile("long=([0-9\\.]+?)&lat=([0-9\\.]+?)'")
	matches := regex.FindAllStringSubmatch(onclick, -1)

	if len(matches) == 0 || len(matches[0]) != 3 {
		return nil, fmt.Errorf("Can't find lat and lng from query: %s", query)
	}

	x, _ := strconv.ParseFloat(matches[0][1], 64)
	y, _ := strconv.ParseFloat(matches[0][2], 64)

	location := LocationInfo{
		Name: s.Text(),
		Lng:  x,
		Lat:  y,
	}

	locations = append(locations, location)

	return locations, nil
}

// Get location (name, lat and lng)
// via StreetDirectory
func getLocationInfo(query string) ([]LocationInfo, error) {
	qs := SDApiQuery{
		Mode:    "search",
		Act:     "all",
		Output:  "json",
		Limit:   1,
		Country: "sg",
		Profile: "template_1",
		Q:       fmt.Sprintf("%s nus", query),
	}

	res, err := goreq.Request{
		Uri:         "http://www.streetdirectory.com/api/",
		QueryString: qs,
		Compression: goreq.Gzip(),
		ShowDebug:   true,
	}.Do()
	if err != nil {
		return nil, err
	}

	var resJSON []map[string]interface{}

	err = res.Body.FromJsonTo(&resJSON)
	if err != nil {
		return nil, err
	}

	var locations []LocationInfo

	if resJSON[0]["total"].(float64) > 0 {
		for ind, obj := range resJSON {
			// Skip first result
			if ind == 0 {
				continue
			}

			location := LocationInfo{
				Name: obj["t"].(string),
				Lng:  obj["x"].(float64),
				Lat:  obj["y"].(float64),
			}

			locations = append(locations, location)
		}
	}

	return locations, nil
}

// Get map PNG from StreetDirectory
func getLocationMap(location LocationInfo) (*telebot.Photo, error) {
	filepath := fmt.Sprintf("%s/%f_%f.png", MAP_CACHE_DIR, location.Lng, location.Lat)

	// if map does not exist in our cache, retrieve!
	if _, err := os.Stat(filepath); err != nil {
		qs := SDMapQuery{
			Level: 14,
			SizeX: 500,
			SizeY: 500,
			Lon:   location.Lng,
			Lat:   location.Lat,
			Star:  1,
		}

		res, err := goreq.Request{
			Uri:         "http://www.streetdirectory.com/api/map/world.cgi",
			QueryString: qs,
			Compression: goreq.Gzip(),
		}.Do()
		if err != nil {
			return nil, err
		}

		data, err := res.Body.ToString()
		if err != nil {
			return nil, err
		}

		err = ioutil.WriteFile(filepath, []byte(data), 0644)
		if err != nil {
			return nil, err
		}
	}

	file, err := telebot.NewFile(filepath)
	if err != nil {
		return nil, err
	}

	thumbnail := telebot.Thumbnail{
		File:   file,
		Width:  500,
		Height: 500,
	}

	photo := telebot.Photo{
		Thumbnail: thumbnail,
		Caption:   location.Name,
	}

	return &photo, nil
}
