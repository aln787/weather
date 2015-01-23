package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	//"strconv"
	"strings"
	"time"
)

// type weatherData struct {
// 	Name string `json:"name"`
// 	Main struct {
// 		Kelvin float64 `json:"temp"`
// 	} `json:"main"`
// }

type weatherProvider interface {
	temperature(city string) (float64, string, string, error)
}

type openWeatherMap struct{}

type multiWeatherProvider []weatherProvider

func (w openWeatherMap) temperature(city string) (float64, string, string, error) {
	requestString := "http://api.openweathermap.org/data/2.5/weather?q=" + city
	log.Println(requestString)
	resp, err := http.Get(requestString)
	if err != nil {
		return 0, "", "", err
	}

	defer resp.Body.Close()

	//Issue with capitalization
	var d struct {
		Coords struct {
			Lat float64 `json:"lat"`
			Lon float64 `json:"lon"`
		} `json:"coord"`
		Main struct {
			Kelvin   float64 `json:"temp"`
			Pressure float64 `json:"pressure"`
		} `json:"main"`
		Name string `json:"name"`
	}
	log.Println(&resp.Body)
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return 0, "", "", err
	}
	log.Printf("Kelvin: %f\nPressure: %f\nName: %s\nLat: %v\nLong: %v", d.Main.Kelvin, d.Main.Pressure, d.Name, d.Coords.Lat, d.Coords.Lon)
	log.Printf("Lat: %f", d.Coords.Lat)

	//lat := strconv.FormatFloat(d.Coordinates.lat, 'f', 4, 32)
	//long := strconv.FormatFloat(d.Coordinates.lon, 'f', 4, 32)
	log.Printf("openWeatherMap: %s: %.2f", city, d.Main.Kelvin)
	return d.Main.Kelvin, "", "", nil
}

func (w forcastIO) temperature(city string, lat string, long string) (float64, error) {
	requestString := "https://api.forecast.io/forecast/" + w.apiKey + "/" + lat + "," + long
	log.Printf(requestString)
	resp, err := http.Get(requestString)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var d struct {
		Currently struct {
			Ferinheight float64 `json:"temperature"`
		} `json:"currently"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return 0, err
	}

	kelvin := (d.Currently.Ferinheight + 459.67) * 5 / 9
	log.Printf("Forcast.io: %s: %.2f", city, kelvin)
	return kelvin, nil
}

type weatherUnderground struct {
	apiKey string
}

type forcastIO struct {
	apiKey string
}

type f struct {
	Observation struct {
		Celsius  float64 `json:"temp_c"`
		location struct {
			lat float64 `json:"latitude"`
			lon float64 `json:"longitude"`
		} `json:"display_location"`
	} `json:"current_observation"`
}

func (w weatherUnderground) temperature(city string) (float64, string, string, error) {
	resp, err := http.Get("http://api.wunderground.com/api/" + w.apiKey + "/conditions/q/" + city + ".json")
	if err != nil {
		return 0, "", "", err
	}

	defer resp.Body.Close()

	// type d struct {
	// 	Observation struct {
	// 		Celsius  float64 `json:"temp_c"`
	// 		location struct {
	// 			lat float64 `json:"latitude"`
	// 			lon float64 `json:"longitude"`
	// 		} `json:"display_location"`
	// 	} `json:"current_observation"`
	// }

	weatherResults := f{}
	body, err := ioutil.ReadAll(resp.Body)
	err = json.Unmarshal(body, &weatherResults)
	if err != nil {
		return 0, "", "", err
	}
	log.Printf("Logging other struct %+v", weatherResults)

	// if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
	// 	return 0, "", "", err
	// }

	//kelvin := d.Observation.Celsius + 273.15
	//lat := strconv.FormatFloat(d.Observation.location.lat, 'f', 6, 64)
	//lon := strconv.FormatFloat(d.Observation.location.lon, 'f', 6, 64)
	//log.Printf("weatherUnderground: %s: %.2f", city, kelvin)
	//return kelvin, lat, lon, nil
	return 0, "", "", nil
}

func (w multiWeatherProvider) temperature(city string) (float64, string, string, error) {
	// Make a channel for temperatures, and a channel for errors.
	// Each provider will push a value into only one.
	temps := make(chan float64, len(w))
	lats := make(chan string, 2)
	longs := make(chan string, 2)
	errs := make(chan error, len(w))

	// For each provider, spawn a goroutine with an anonymous function.
	// That function will invoke the temperature method, and forward the response.
	for _, provider := range w {
		go func(p weatherProvider) {
			k, lat, long, err := p.temperature(city)
			if err != nil {
				errs <- err
				return
			}
			//log.Printf("add to channel")
			temps <- k
			lats <- lat
			longs <- long
			//log.Printf("after channel addition")
		}(provider)
		//log.Printf("out of go routine")
	}
	//log.Printf("out of for loop")

	sum := 0.0

	for i := 0; i < len(w); i++ {
		//log.Printf("Other for statement")
		lonVal := <-longs
		//log.Printf("after long")
		latVal := <-lats
		//log.Println("Lat: " + latVal)
		//log.Println("Lon: " + lonVal)
		if len(latVal) > 5 && len(lonVal) > 5 {
			fio := forcastIO{apiKey: "0e5fb5519fd640307928245167e0e424"}
			forcastTemp, err := fio.temperature(city, latVal, lonVal)
			if err != nil {
				return 0, "", "", err
			}
			sum += forcastTemp
		}
	}

	// Collect a temperature or an error from each provider.
	for i := 0; i < len(w); i++ {
		select {
		case temp := <-temps:
			sum += temp
		case err := <-errs:
			return 0, "", "", err
		}
	}

	// Return the average, same as before.
	return sum / float64(len(w)+1), "", "", nil
}

func main() {
	mw := multiWeatherProvider{
		openWeatherMap{},
		weatherUnderground{apiKey: "af20b75c5ffafa39"},
	}

	http.HandleFunc("/weather/", func(w http.ResponseWriter, r *http.Request) {
		begin := time.Now()
		city := strings.SplitN(r.URL.Path, "/", 3)[2]

		temp, _, _, err := mw.temperature(city)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"city": city,
			"temp": temp,
			"took": time.Since(begin).String(),
		})
	})
	http.ListenAndServe(":8080", nil)
}
