package main

	// Use gpiocdev to access RPi GPIO pins for SM-420 Vibration Sensor
	// NOTE: GPIO pin identification and the GPIO library.
	//	Broadcom numbers (recommended and as used)
	//		GPIO numbers used by the manufacturer and are the ones used by the operating system
	//	Pin numbers
	//		If a GPIO is routed out to the expansion header it may sometimes be referred to by pin number
	//		E.g. GPIO 4 is connected to pin 7
	//	Wiring Pi numbers
	//		These are the numbers used to identify the GPIO when using the wiringPi library
	//		Probably introduced as similar to Arduino idents.
	//
	// See example code: https://github.com/warthog618/go-gpiocdev/blob/master/examples/watch_line_value/main.go
	//
	// Build with (compatibility):
	//	env GOOS=linux GOARCH=arm GOARM=6 go build -ldflags "-X main.Version=0.0.1.33" -o vibration
	//
	// There remains an unresolved issue with running under systemd. for now running with:
	//	sudo nohup ./vibration &

import (
	log "github.com/sirupsen/logrus"
	"github.com/warthog618/go-gpiocdev"
	"time"
	"os"
	"fmt"
	"syscall"
	"bufio"
	"strings"
	"strconv"
	"github.com/robfig/cron/v3"
	"net/http"
)

//	General globals and constants
var	Version			= "development"		// Replaced during build: ldflags "-X main.Version=0.0.1.21"
var	filePath		= "/var/lib/vibration/"
var	vibrationDir	= "/var/lib/vibration/"
var	vibrationFile	= "vibration.txt"
var	htmlFile		= "index.html"
var	timeLayout		= "2006-01-02 15:04:05"
var	DailyFile		*os.File
var	err				error

//	GPIO Configuration
var	chip		= "gpiochip0"
var	offset1		= 17					// GPIO 17 on pin 11
var	offset2		= 27					// GPIO 27 on pin 13, not seeing anything here
var	debounceMS	time.Duration	= 375	// debounce millisecs, 75, 150, 250 did ringing

//	Timer to delay action while vibration sensor reacts to event
var	timerDelay	*time.Timer
var	pauseDur	time.Duration		= 30*time.Second	// wait time after event
//	Timer to super-debounce events
var	timerDeBounce	*time.Timer
var	superDebounce	time.Duration	= 2*time.Second		// longest debounce ring case
var	inDeBounce		bool			= false
var	iDebounceCnt	int				= 0

// areDevicesPaired func returns boolean
//	Compare two device names, return true if same device and one is rising, other falling
func areDevicesPaired( device1 string, device2 string ) bool {
	dev1Slice := []rune(device1)
	dev2Slice := []rune(device2)
	len1 := len(device1)
	len2 := len(device2)
	// Same GPIO?
	//	Below works for single or two digit device #s
	sameGPIO := dev1Slice[len1-1] == dev2Slice[len2-1] && dev1Slice[len1-2] == dev2Slice[len2-2]
	// Make sure we have Rising then Fallng
	riseFall := dev1Slice[0]=='r' && dev2Slice[0]=='f'
	return sameGPIO && riseFall
}	// areDevicesPaired

//	string2Time converts local time string to time.Time value
func string2Time( text string ) time.Time {
	//log.Error( "vibration.string2Time - input: " + text )
	temp	:= text[44:]							// Odd, strings.Index ignores substring in argument
	pos		:= strings.Index( temp, "T" )			// Locate the 2nd date start, start after 1st
	timeStr	:= temp[pos-10:pos+15]					// Remove the "T" from time to match pattern
	runeSlice := []rune(timeStr)					// Yes, this is how it is done
	runeSlice[10] = ' '								// replace the T with a space
	timeStr	= string(runeSlice)						// Now Parse works
	theTime, err := time.Parse(timeLayout,timeStr )	// Documentation says fractional second is permitted
	if ( err != nil ) {
		log.Error( "vibration.string2Time - parse error, time:" + timeStr )
	}
	return theTime
}	// string2Time

// makeTableDatafiles func
//	Create a file of HTML as a table,one row per line of text
func makeTableDatafiles( dataFileName, tableFileName, note string) {
	var	prevDate	= "2025-01-01"
	var	tempStr		= ""
	var	count		= 0
	log.Error( "vibration.makeTableDatafiles - " + tableFileName + " " + note )
	// Open the vibration.txt file
	// Create an HTML table file of four columns (date&time, R/F, details, day count).
	dataTable, err := os.OpenFile( tableFileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Error("vibration.makeTableDatafiles - " + dataFileName + "-->" + tableFileName + " create Failure.")
		return
	}
	timeStr := time.Now().Format(timeLayout)
	text := fmt.Sprintf( " Vsn: %s at %s", Version, timeStr )
	dataTable.WriteString( "<!-- infinitive.makeTableDatafiles():" + text + " -->\n" )
	dataTable.WriteString( "<!DOCTYPE html>\n<html lang=\"en\">\n" )
	dataTable.WriteString( "<head>\n<title>Vibrations Detected " + text + "</title>\n" )
	dataTable.WriteString( "<style>\n td {\n  text-align: left;\n  }\n table, th, td {\n  border: 1px solid;\n  border-spacing: 5px;\n  border-collapse: collapse;\n }\n</style>\n</head>\n" )
	dataTable.WriteString( "<body>\n<h2>Vibrations Detected " + text + "</h2>\n" )
	dataTable.WriteString( "<table width=\"920\">\n" )
	dataTable.WriteString( "<tr><td>Time</td><td>Event</td><td>Description</td>" )
	// Process the vibration file.
	vibFile, err := os.Open( dataFileName )
	if err != nil {
		log.Fatal("vibration.makeTableDatafiles -unable to read " + dataFileName + ", err=", err)
		return
	}
	scanFile := bufio.NewScanner(vibFile)
	for scanFile.Scan() {
		text := scanFile.Text()
		if prevDate[:10] == text[:10] {					// Same date...
			dataTable.WriteString("<td></td></tr>\n")	// row is a date match, previous row is empty cell
		} else {
			if count == 0 {
				tempStr = ""
			} else {
				tempStr = strconv.Itoa(count)
			}
			dataTable.WriteString("<td>" + tempStr + "</td></tr>\n")	// row is a different date, show counts
			prevDate = text[:10]
			count = 0
		}
		if ( len(text) < 31 ) {
			dataTable.WriteString( "    <tr><td>" + text[:19] + "</td><td>" + text[20:30] + "</td><td> ? </td></tr>\n" )
		} else {
			dataTable.WriteString( "    <tr><td>" + text[:19] + "</td><td>" + text[20:30] + "</td><td>" + text[31:] + "</td>" )
		}
		count++
	}	// Done with file
	dataTable.WriteString( "<td>" + strconv.Itoa(count) + "</td></tr>\n</table>\n</body>\n" )
	dataTable.WriteString( "</html>\n\n" )
	dataTable.Close()
	return
}	// makeTableDatafiles

// resetTimer stops, drains and resets the timer.
//	Ref: https://antonz.org/timer-reset/
func resetTimer(t *time.Timer, d time.Duration) {
	if !t.Stop() {
		select {
			case <-t.C:
		default:
		}
	}
	t.Reset(d)
}	// resetTimer

//	Event Handler captures vibration events
// GPIO code derived from https://github.com/warthog618/go-gpiocdev/blob/master/examples/watch_line_value/main.go
func eventHandler(evt gpiocdev.LineEvent) {
	var edge	string	= "rising"

	if inDeBounce {
		//log.Error("vibration.eventHandler - inDeBounce true - skipping event")
		iDebounceCnt++
	} else {
		inDeBounce = true								//	Skip until timer expires...
		resetTimer( timerDeBounce, superDebounce )		//	Reset the timer
		if evt.Type == gpiocdev.LineEventFallingEdge {
			edge = "falling"
		}
		dt := time.Now()
		temp := fmt.Sprintf("%-7s%3d event: #%d(%d) %s (%s)",
			edge,
			evt.Offset,
			evt.Seqno,
			evt.LineSeqno,
			dt.Format(time.RFC3339Nano),
			evt.Timestamp)
		messg := fmt.Sprintf( "%s %s\n", dt.Format("2006-01-02T15:04:05"), temp )
		_, err := DailyFile.WriteString( messg )
		if err != nil {
			log.Error( "vibration.eventHandler - Error in WriteStrig - " + temp )
		} else {
			// Revised, should match file entry nohup or systemd...
			log.Error( "vibration.eventHandler >> " + temp )
		}
	}
	// Reset the timer used to trigger new HTML creation (reset isn't as easy as described)
	//		resetTimer begins (is reset) for each GPIO event
	resetTimer( timerDelay, pauseDur )
	resetTimer( timerDeBounce, superDebounce )
}	// eventHandler

func main() {
	log.Error("vibration.go - " + Version)

    // 1.1 Set up 1st GPIO with a handler function on state transitions, 1st of 2
	l, err := gpiocdev.RequestLine(
		chip,
		offset1,
		gpiocdev.WithPullUp,
		gpiocdev.WithBothEdges,
		gpiocdev.WithEventHandler(eventHandler) )
	if err != nil {
		if err == syscall.Errno(22) {
			log.Error("vibration.main 1.1 - RequestLine error 22")
		} else {
		log.Error("vibration.main 1.2 - RequestLine returned error: %s", err)
		// os.Exit(1)           -- removed, under systemd this would restart forever.
		}
	}
	// 1.2 Set up the 2nd GPIO, 2nd of 2
	l, err = gpiocdev.RequestLine(
		chip,
		offset2,
		gpiocdev.WithPullUp,
		gpiocdev.WithBothEdges,
		gpiocdev.WithEventHandler(eventHandler) )
	if err != nil {
		if err == syscall.Errno(22) {
			log.Error("vibration.main 1.1 - Note: the WithPullUp option requires Linux 5.5 or later - check kernel version.")
		} else {
		log.Error("vibration.main 1.2 - RequestLine returned error: %s", err)
		// os.Exit(1)		-- removed, under systemd this would restart forever.
		}
	}
	debounce := debounceMS * time.Millisecond
	l.Reconfigure( gpiocdev.WithDebounce(debounce) )
	defer l.Close()
	log.Error("vibration.main 1.5 - GPIO configured.")

	// 2. Update the HTML file on a restart.
	makeTableDatafiles( filePath+vibrationFile, vibrationDir+htmlFile, "on start.")

	// 3. In folder filePath, Open append vibrationFile for append - /var/lib/vibration/vibration.txt ... 
	DailyFile, err = os.OpenFile(filePath+vibrationFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664 )
	if err != nil {
		// Would a log.Fatal here cause infinite systemd restarts?
		log.Error( "vibration.main 3 - DailyFile=os.OpenFile(" + filePath+vibrationFile + ") - Error")
	} else {
		log.Error( "vibration.main 3 - DailyFile-os.OpenFile(" + filePath+vibrationFile + ") - Success." )
	}

	// 4. Set up cron 1 to update the html table file 9 times per day at hours specified
	cronJob1 := cron.New(cron.WithSeconds())
	cronJob1.AddFunc( "2 0 6,8,10,12,14,16,18,20,22 * * *", func () {
		log.Error("vibration.go - 4 cron - Prepare the html table - makeTableDatafiles().")
		makeTableDatafiles( filePath+vibrationFile, vibrationDir+htmlFile, "cron")
	} )
	cronJob1.Start()

	// 5. Start static file server for vibration HTML file
	log.Error("vibration.main 5.1 - start FileServer() for vibration data.")
	go func() {
		// Simple static FileServer
		fs := http.FileServer(http.Dir(filePath[:len(filePath)-1]))	// Remove trailing directory "/"
		http.Handle("/html/", http.StripPrefix("/html/", fs))		// Is this right?
		err:= http.ListenAndServe(":8081", nil)				// localhost:8081/vibration/index.html
		if err != nil {
			log.Error("vibration.main 5.2 - Static File Server ListenAndServe Failed. ", err)
		}
   	} ()

	// 6. Ceate and stop timers, the the delay timer for events and suoper-DeBounce
	timerDelay	= time.NewTimer( pauseDur )
	if !timerDelay.Stop() {
		log.Error("vibration.main 6 - timerDelay in unexpected state.")
	}
	timerDeBounce = time.NewTimer( superDebounce )
	if !timerDeBounce.Stop() {
		log.Error("vibration.main 6 - timerDeBounce in unexpected state.")
	}

	// 7. The eventhandler for the GPIO handles event logging.
	//	Adding the timerDelay.C action broke "select {}" forever loop.
	//	"for" with sleep is a crude fix to the problem.
	for {
		select {
			case <-timerDelay.C:
				// We have timeout, update the table
				makeTableDatafiles( filePath+vibrationFile, vibrationDir+htmlFile, "on delay")
			case <- timerDeBounce.C:
				log.Error("vibration.main 7 - timerDeBounce exit, count=" + strconv.Itoa(iDebounceCnt))
				inDeBounce = false
				iDebounceCnt = 0
		}
		time.Sleep( 2*time.Second)		// loop pause
	}
	log.Error("vibration.main 7 - unexpected normal termination.")
} // main - vibration.go
