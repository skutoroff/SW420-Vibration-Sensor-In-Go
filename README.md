# W420-Vibration-Sensor-In-Go

After finishing our basement with a half-bathroom, observed the effluent pump shook the exit pipe hard.
Began to wonder about sensing the vibration and the idea became this project.

Found a cheap $0.95 vibration sensor on Adafruit and even bought prototyping boards, wires, and resistors to hook one up.
Then found the SW-420 sensor modules on Amazon with 5 of them for less than $7.00.
These have the sensor, circuitry, status LEDs, and a three wire interface suitable for the GPIO bus.
Wired one up to a Raspberry Pi Zero2W  to which I had soldered a GPIO header.
Then I searched for a software library and found github.com/warthog618/go-gpiocdev

Attached the Pi and the SW-420 to the plumbing with wire ties and began the coding.
As done in my project https://github.com/skutoroff/Infinitive-Carrier-HVAC-Enhanced, the program would serve a generated static web page of the data.

## Getting started

The RPiZ2WH was already being used as a PiHole.
It just had to be moved to the closet housing the effluent sump and plumbing.
The code as developed accepts triggered events and appends an event record to file vibration.txt.
Some effort solved the signal debounce issue, the software debounce is more effective than the SW-420 sensitivity adjustment.
The on SW-420 board adjustment has a non-specific effect on debounce, possibly better for sensitivity.
These board also may be prone to failure.

The project is still in development.
Current confounded by the sensor sometimes missing activations.


To be continued.
