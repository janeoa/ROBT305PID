package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/gosuri/uilive"
	"github.com/olekukonko/tablewriter"
)

const printTheFileLogs = false
const period = 5000
const kp = 1 //float64(period) / 360.0 / 1000
const ki = 0
const kd = 0 //1000

var data [][]string

func main() {
	initMotor()

	relativeZero := getTicks()

	var aim float64
	// aim = 90 //deg
	arg := os.Args[1]
	aim, _ = strconv.ParseFloat(arg, 10)
	// setPWM("pwm-1:0", period, constrain(1, 0, 1), true)
	go pid(relativeZero, aim)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		setPWM("pwm-1:1", period, 0, false)
		os.Exit(0)
	}()

	go draw()

	time.Sleep(time.Second * 10)
}

func getTicks() int {
	dat, err := ioutil.ReadFile("/sys/devices/platform/ocp/48304000.epwmss/48304180.eqep/position")
	dat = bytes.TrimSpace(dat)
	check(err)
	//fmt.Println(hex.Dump(dat))
	flt, _ := (strconv.Atoi(string(dat)))

	return flt
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func setDirection(forward bool) {
	// if !left {
	if forward {
		writeToFile("/sys/class/gpio/gpio66/value", "0")
		writeToFile("/sys/class/gpio/gpio69/value", "1")
	} else {
		writeToFile("/sys/class/gpio/gpio66/value", "1")
		writeToFile("/sys/class/gpio/gpio69/value", "0")
	}
	// }
}

func pid(zero int, aim float64) {

	integral := 0.0
	newtime := time.Now().Unix()
	oldtime := time.Now().Unix()
	for {
		newtime = time.Now().Unix()
		tick := getTicks()
		diff := aim - tickToDeg(tick-zero)
		setDirection(diff > 0)
		//TODO if diff>180 then -180
		integral = integral + ki*diff*float64(newtime-oldtime)
		motorPower := math.Abs(float64(kp*diff) + integral + kd*float64(newtime-oldtime)) //
		setPWM("pwm-1:1", period, constrain(motorPower, 0, 1), true)
		// fmt.Printf("aim: %2.2f\tticks: %d\tdiff: %2.2f\tmotorPower: %2.2f\n", aim, tick, diff, motorPower)
		// fmt.Fprintf(writer, "aim: %2.2f\tticks: %d\tdiff: %2.2f\ttime: %d\n\tintegral: %2.2f\tmotorPower: %2.2f\r", aim, tick, diff, newtime-oldtime, integral, motorPower)
		data = [][]string{
			[]string{"aim", fmt.Sprintf("%2.2f", aim)},
			[]string{"tick", fmt.Sprintf("%d", tick)},
			[]string{"diff", fmt.Sprintf("%2.2f", diff)},
			[]string{"time", fmt.Sprintf("%d", newtime-oldtime)},
			[]string{"inte", fmt.Sprintf("%2.2f", integral)},
			[]string{"powe", fmt.Sprintf("%2.2f", motorPower)},
		}

		oldtime = newtime
		time.Sleep(time.Millisecond * 10)
	}

}

func draw() {
	writer := uilive.New()
	// start listening for updates and render
	writer.Start()
	for {
		tableString := &strings.Builder{}
		table := tablewriter.NewWriter(tableString)
		table.SetHeader([]string{"var", "__value__"})

		for _, v := range data {
			table.Append(v)
		}
		table.Render() // Send output
		fmt.Fprintf(writer, tableString.String())
		time.Sleep(time.Millisecond * 250)
	}
	fmt.Fprintln(writer, "Finished: Downloaded 100GB")
	writer.Stop() // flush and stop rendering
}

func constrain(val float64, from float64, to float64) float64 {
	if val > to {
		val = to
	}
	if val < from {
		val = from
	}
	return val
}

func tickToDeg(tiks int) float64 {
	/**
	3rev - 1000tick
	3*360 - 1000
	y     - x
	*/
	return float64(tiks) * (3.0 * 360.0) / 1000.0
}

func initMotor() {
	color.HiYellow("CMD")
	//config-pin -a p9.22 pwm
	cmd := exec.Command("config-pin", "-a", "p9.22", "pwm")
	//cmd := exec.Command("sleep", "1")
	log.Printf("Running command and waiting for it to finish...")
	err := cmd.Run()

	if err != nil {
		log.Printf("PWM ERROR: %v", err)
	} else {
		color.Green("PWM Mode is set")
	}

	cmd = exec.Command("config-pin", "-a", "p8.11", "qep")
	//cmd := exec.Command("sleep", "1")
	log.Printf("Running command and waiting for it to finish...")
	err = cmd.Run()
	cmd = exec.Command("config-pin", "-a", "p8.12", "qep")
	err2 := cmd.Run()

	if (err != nil) || (err2 != nil) {
		log.Printf("eQEP ERROR: %v and %v", err, err2)
	} else {
		color.Green("eQEP is set")
	}

	cmd = exec.Command("config-pin", "-a", "p8.8", "gpio")
	cmd.Run()
	cmd = exec.Command("config-pin", "-a", "p8.10", "gpio")
	cmd.Run()

	writeToFile("/sys/class/gpio/gpio67/direction", "out")
	writeToFile("/sys/class/gpio/gpio68/direction", "out")

	writeToFile("/sys/class/gpio/gpio67/value", "1")
	writeToFile("/sys/class/gpio/gpio68/value", "0")

	color.Green("GPIO mode set\n")

	setPWM("pwm-1:1", period, 0, false)

	color.Green("PWM parameters set\n")

}

func setPWM(channel string, period int, dutyCycle float64, enable bool) {
	if dutyCycle > 1 {
		panic("period < dutyCycle")
	}

	dutyCycle = dutyCycle * float64(period)

	writeToFile(fmt.Sprintf("/sys/class/pwm/%s/period", channel), fmt.Sprintf("%d", period))
	writeToFile(fmt.Sprintf("/sys/class/pwm/%s/duty_cycle", channel), fmt.Sprintf("%d", int(dutyCycle)))

	if enable {
		writeToFile(fmt.Sprintf("/sys/class/pwm/%s/enable", channel), "1")
	} else {
		writeToFile(fmt.Sprintf("/sys/class/pwm/%s/enable", channel), "0")
	}
}

func writeToFile(path string, value string) {
	f, err := os.Create(path)
	if err != nil {
		fmt.Println(err)
		return
	}
	_, err = f.WriteString(value)
	if err != nil {
		fmt.Println(err)
		f.Close()
		return
	}
	if printTheFileLogs {
		color.Green("SET: %s TO FILE %s", value, path)
	}
	err = f.Close()
	if err != nil {
		fmt.Println(err)
		return
	}
}
