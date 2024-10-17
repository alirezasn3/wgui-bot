package main

import (
	"fmt"
	"strconv"
	"strings"
)

type IPAddress struct {
	Octets [4]int
}

func (a *IPAddress) Increment() {
	if a.Octets[3] < 254 {
		a.Octets[3]++
	} else {
		a.Octets[3] = 1
		if a.Octets[2] < 254 {
			a.Octets[2]++
		} else {
			a.Octets[2] = 1
			if a.Octets[1] < 254 {
				a.Octets[1]++
			} else {
				a.Octets[1] = 1
				if a.Octets[0] < 254 {
					a.Octets[0]++
				} else {
					panic(fmt.Sprintf("cant increment address, %d", a.Octets))
				}
			}
		}
	}
}

func (a *IPAddress) ToString() string {
	return fmt.Sprintf("%d.%d.%d.%d", a.Octets[0], a.Octets[1], a.Octets[2], a.Octets[3])
}

func (a *IPAddress) Parse(address string) error {
	serverNetworkAddressOctets := strings.Split(address, ".")
	var err error
	for i, o := range serverNetworkAddressOctets {
		a.Octets[i], err = strconv.Atoi(o)
		if err != nil {
			return err
		}
	}
	return nil
}
