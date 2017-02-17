package main

import (
	"bytes"
	"fmt"
	"github.com/stianeikeland/go-rpio"
	"html/template"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var num = 0

const bootTime = 55 // Seconds
const pinPower = 9
const pinBrewer = 10
const pinStatus = 11
const pinBeeper = 5

var (
	power  rpio.Pin
	brewer rpio.Pin
	status rpio.Pin
	beeper rpio.Pin

	booting = false

	homepage, _ = template.ParseFiles("homepage.html")
)

type Homepage struct {
	Num     int
	Message string
}

func push(pin rpio.Pin, ms uint) bool {
	if booting {
		fmt.Println("rejecting push, because booting")
		return false
	}
	if pin == brewer {
		writeCoffeePoint()
		go func() {
			booting = true
			time.Sleep(15 * time.Second)
			booting = false
			go func() {
				for i := 0; i < 2; i++ {
					time.Sleep(300 * time.Millisecond)
					beeper.High()
					time.Sleep(300 * time.Millisecond)
					beeper.Low()
				}
			}()
		}()
	}
	go func() {
		pin.High()
		status.Low()
		time.Sleep(time.Duration(ms) * time.Millisecond)
		pin.Low()
		status.High()
	}()
	return true
}

func brew(w http.ResponseWriter, r *http.Request) {
	fmt.Println("pressing brew button")
	if push(brewer, 300) {
		num += 1
		w.Header().Set("Location", fmt.Sprintf("/n%d", num))
	} else {
		w.Header().Set("Location", "/")
	}
	w.WriteHeader(307)
}

func start(w http.ResponseWriter, r *http.Request) {
	fmt.Println("pressing power button")
	if push(power, 300) {
		w.Header().Set("Location", "/s")
	} else {
		w.Header().Set("Location", "/")
	}
	w.WriteHeader(307)
}

func both(w http.ResponseWriter, r *http.Request) {
	fmt.Println("pressing power and brew button")
	go func() {
		if !push(power, 300) {
			w.Header().Set("Location", "/")
			return
		}
		booting = true

		blink := time.Tick(1 * time.Second)
		sleep := time.After(bootTime * time.Second)
	blinking:
		for {
			select {
			case <-sleep:
				fmt.Println("machine booted")
				break blinking
			case <-blink:
				status.Toggle()
				break
			}
		}

		booting = false
		push(brewer, 300)
		num += 1
	}()
	w.Header().Set("Location", "/b")
	w.WriteHeader(307)
}

func writeCoffeePoint() {
	// data.csv:
	// [ timestamp, unix-time-code ]
	now := time.Now()
	f, err := os.OpenFile("/home/pi/data.csv", os.O_WRONLY|os.O_APPEND|os.O_CREATE, os.ModePerm)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s, %d\n", now, now.Unix())
}

func home(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	var stat = "no status message"
	var req = strings.Split(r.URL.Path, "/")
	if len(req) == 2 {
		if len(req[1]) != 0 {
			switch req[1][0] {
			case 's':
				stat = "Starting coffee machine"
				break
			case 'n':
				stat = fmt.Sprintf("Brewing coffee number #%s", req[1][1:])
				break
			case 'b':
				stat = fmt.Sprintf("Now booting the machine, will reject requests for the next %d Seconds", bootTime)
				break
			}
		}
	}
	// debug:
	homepage, _ := template.ParseFiles("homepage.html")
	homepage.Execute(w, Homepage{Num: num, Message: stat})
}

func data(w http.ResponseWriter, r *http.Request) {
	req := strings.Split(r.URL.Path, "/")
	if len(req) < 3 {
		return
	}
	switch req[2] {
	case "data.csv":
		f, e := os.Open("data.csv")
		if e == nil {
			io.Copy(w, f)
			f.Close()
		}
		break
	}
}

func lineCounter(r io.Reader) (int, error) {
	buf := make([]byte, 32*1024)
	count := 0
	lineSep := []byte{'\n'}

	for {
		c, err := r.Read(buf)
		count += bytes.Count(buf[:c], lineSep)

		switch {
		case err == io.EOF:
			return count, nil

		case err != nil:
			return count, err
		}
	}
}

func main() {
	f, ferr := os.Open("data.csv")
	if ferr != nil {
		num = 0
	} else {
		num, _ = lineCounter(f)
		f.Close()
	}
	err := rpio.Open()
	if err != nil {
		fmt.Printf("Failed to open GPIO: %s", err)
		return
	}
	defer rpio.Close()

	power = rpio.Pin(pinPower)
	brewer = rpio.Pin(pinBrewer)
	status = rpio.Pin(pinStatus)
	beeper = rpio.Pin(pinBeeper)

	power.Output()
	brewer.Output()
	status.Output()
	beeper.Output()

	status.High()

	go func() {
		beeper.High()
		time.Sleep(time.Second)
		beeper.Low()
	}()

	http.HandleFunc("/", home)
	http.HandleFunc("/brew/", brew)
	http.HandleFunc("/start/", start)
	http.HandleFunc("/both/", both)
	http.HandleFunc("/data/", data)
	http.ListenAndServe(":54773", nil)
}
