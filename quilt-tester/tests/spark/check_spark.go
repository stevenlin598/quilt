package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

func main() {
	output, err := exec.Command("quilt", "containers").Output()
	if err != nil {
		panic(err)
	}
	containerStr := string(output)
	containers := strings.Split(containerStr, "\n")

	var stitchID string
	for _, line := range containers {
		if strings.Contains(line, "run master") {
			matches := regexp.MustCompile("StitchID *.? ([0-9]+)").FindStringSubmatch(line)
			if len(matches) != 2 {
				fmt.Println("No stitchID found")
				os.Exit(1)
			}
			stitchID = matches[1]
		}
	}
	if stitchID == "" {
		fmt.Println("FAILED, no spark master node was found.")
		os.Exit(1)
	}
	fmt.Printf("StitchID = |%s|", stitchID)
	output, err = exec.Command("quilt", "logs", stitchID).CombinedOutput()

	if err != nil {
		panic(err)
	}
	log := string(output)
	fmt.Printf(log)
	if !strings.Contains(log, "Pi is roughly") {
		fmt.Println("FAILED, sparkPI did not execute correctly.")
	} else {
		fmt.Println("PASSED")
	}

}
