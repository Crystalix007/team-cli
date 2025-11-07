package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func promptBool(msg string) (bool, error) {
	for {
		line, err := prompt(msg)
		if err != nil {
			return false, err
		}

		switch line {
		case "y", "yes", "t":
			return true, nil
		case "n", "no", "f", "q", "quit", "s", "stop", "e", "exit":
			return false, nil
		}
	}
}

func promptSelection(msg string, min int, max int) (int, error) {
	for {
		line, err := prompt(msg)
		if err != nil {
			return 0, err
		}

		val, err := strconv.Atoi(line)
		if err != nil {
			continue
		}

		if val < min || val > max {
			continue
		}

		return val, nil
	}
}

func promptTime(msg string) (time.Time, error) {
	for {
		line, err := prompt(msg)
		if err != nil {
			return time.Time{}, err
		}

		if strings.EqualFold(line, "now") || line == "" {
			return time.Time{}, nil
		}

		val, err := time.ParseInLocation(time.DateTime, line, time.Local)
		if err != nil {
			continue
		}

		return val, nil
	}
}

func promptString(msg string) (string, error) {
	for {
		line, err := prompt(msg)
		if err != nil {
			return "", err
		}

		if line == "" {
			continue
		}

		return line, nil
	}
}

var ioReader *bufio.Reader

func prompt(msg string) (string, error) {
	fmt.Print(msg)

	if ioReader == nil {
		ioReader = bufio.NewReader(os.Stdin)
	}

	input, err := ioReader.ReadString('\n')
	if err != nil {
		return "", err
	}

	input = strings.TrimSpace(input)

	return input, nil
}
