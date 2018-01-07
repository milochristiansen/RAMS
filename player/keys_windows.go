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

import "syscall"
import "time"

var user32 = syscall.NewLazyDLL("user32.dll")
var procGAKS = user32.NewProc("GetAsyncKeyState")

// Key codes from MSDN
var keycodes = [4]uint{
	0xb0, // VK_MEDIA_NEXT_TRACK
	0xb1, // VK_MEDIA_PREV_TRACK
	0xb2, // VK_MEDIA_STOP
	0xb3, // VK_MEDIA_PLAY_PAUSE
}

var keys = [4]int{
	KeyNext,
	KeyPrev,
	KeyStop,
	KeyPause,
}

func KeyServer() chan int {
	ch := make(chan int)
	go func(ch chan int) {
		down := [4]bool{false, false, false, false}

		for {
			for i, key := range keycodes {
				// val is not a simple boolean!
				// 0 means "not pressed" (also certain errors)
				// If LSB is set the key was just pressed (this may not be reliable)
				// If MSB is set the key is currently down.
				val, _, _ := procGAKS.Call(uintptr(key))

				// Turn a press into a transition and track key state.
				goingdown := false
				if int(val) != 0 && !down[i] {
					goingdown = true
					down[i] = true
				}
				if int(val) == 0 && down[i] {
					down[i] = false
				}
				if goingdown {
					ch <- keys[i]
				}
			}
			time.Sleep(500 * time.Millisecond)
		}
	}(ch)
	return ch
}
