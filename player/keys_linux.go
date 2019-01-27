/*
Copyright 2017 by Milo Christiansen

This software is provided 'as-is', without any express or implied warranty. In
no event will the authors be held liable for any damages arising from the use of
this software.

Permission is granted to anyone to use this software for any purpose, including
commercial applications, and to alter it and redistribute it freely, subject to
the following restrictions:

1. The origin of this software must not be misrepresented; you must not claim
that you wrote the original software. If you use this software in a product, an
acknowledgment in the product documentation would be appreciated but is not
required.

2. Altered source versions must be plainly marked as such, and must not be
misrepresented as being the original software.

3. This notice may not be removed or altered from any source distribution.
*/

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// parses through the /proc/bus/input/devices file for keyboard devices.
// Copied from `github.com/gearmover/keylogger` with trivial modification.
func dumpDevices() ([]string, error) {
	cmd := exec.Command("/bin/sh", "-c", "/bin/grep -E 'Handlers|EV=' /proc/bus/input/devices | /bin/grep -B1 'EV=120013' | /bin/grep -Eo 'event[0-9]+'")

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(output)

	var devices []string

	for line, err := buf.ReadString('\n'); err == nil; {
		devices = append(devices, "/dev/input/"+line[:len(line)-1])

		line, err = buf.ReadString('\n')
	}

	return devices, nil
}

var keys = map[uint16]int{
	0xa3: KeyNext,
	0xa5: KeyPrev,
	0xa4: KeyPause,
	0xa6: KeyStop,
}

// Most of the code here comes from `github.com/gearmover/keylogger`.
func KeyServer() chan int {
	// drop privileges when executing other programs
	syscall.Setgid(65534)
	syscall.Setuid(65534)

	// dump our keyboard devices from /proc/bus/input/devices
	devices, err := dumpDevices()
	if err != nil {
		panic(err)
	}
	if len(devices) == 0 {
		panic("No input devices found")
	}

	// bring back our root privs
	syscall.Setgid(0)
	syscall.Setuid(0)

	ch := make(chan int)
	var buffer = make([]byte, 24)
	go func(ch chan int) {
		// Open the first keyboard device.
		input, err := os.OpenFile(devices[0], os.O_RDONLY, 0600)
		if err != nil {
			panic(err)
		}
		defer input.Close()

		for {
			// read the input events as they come in
			n, err := input.Read(buffer)
			if err != nil {
				panic(err)
			}

			if n != 24 {
				fmt.Println("Weird Input Event Size: ", n)
				continue
			}

			// parse the input event according to the <linux/input.h> header struct
			binary.LittleEndian.Uint64(buffer[0:8]) // Time stamp stuff I could care less about
			binary.LittleEndian.Uint64(buffer[8:16])
			etype := binary.LittleEndian.Uint16(buffer[16:18])        // Event Type. Always 1 for keyboard events
			code := binary.LittleEndian.Uint16(buffer[18:20])         // Key scan code
			value := int32(binary.LittleEndian.Uint32(buffer[20:24])) // press(1), release(0), or repeat(2)

			if etype == 1 && value == 1 && keys[code] != 0 {
				ch <- keys[code]
			}

			// TODO: Do I need this for sure?
			time.Sleep(500 * time.Millisecond)
		}
	}(ch)
	return ch
}
