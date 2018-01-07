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

import "fmt"
import "sync"
import "net/http"

import "github.com/gorilla/websocket"

var GlobalSockets = &Sockets{
	clients: map[*websocket.Conn]bool{},
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,

	CheckOrigin: func(r *http.Request) bool { return true },
}

type Sockets struct {
	sync.Mutex

	// Used for broadcast.
	clients map[*websocket.Conn]bool
}

type Change int

const (
	ChangeTime Change = iota
	ChangeState
	ChangeNowPlaying
	ChangePlaylist
	ChangeInit // Sent as the first message on any socket as soon as it is opened.
	ChangeStream
)

func (s *Sockets) Broadcast(changed Change, state *statusRes) {
	s.Lock()
	defer s.Unlock()

	msg := socketBroadcast{changed, state}

	for conn := range s.clients {
		err := conn.WriteJSON(msg)
		if err != nil {
			fmt.Println("Socket closed:", err)
			conn.Close()
			delete(s.clients, conn)
		}
	}
}

func (s *Sockets) SendTo(conn *websocket.Conn, changed Change, state *statusRes) {
	s.Lock()
	defer s.Unlock()

	err := conn.WriteJSON(socketBroadcast{changed, state})
	if err != nil {
		fmt.Println("Socket closed:", err)
		conn.Close()
		delete(s.clients, conn)
	}
}

func (s *Sockets) Upgrade(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = conn.WriteJSON(socketBroadcast{ChangeInit, GlobalPlayer.State()})
	if err != nil {
		fmt.Println("Socket closed:", err)
		conn.Close()
		return
	}

	s.Lock()
	s.clients[conn] = true
	s.Unlock()

	for {
		var cmd socketCmd
		err := conn.ReadJSON(&cmd)
		if err != nil {
			fmt.Println("Socket closed:", err)
			s.Lock()
			delete(s.clients, conn)
			s.Unlock()
			break
		}

		switch cmd.Act {
		case 0:
			GlobalPlayer.FakeInput(cmd.State)
		case 1:
			GlobalPlayer.Enqueue(cmd.AutoPlay, cmd.IDs...)
		case 2:
			GlobalPlayer.Dequeue(cmd.Index)
		case 3:
			_ = GlobalPlayer.LoadPlaylist(cmd.Name, false)
		case 4:
			_ = GlobalPlayer.SavePlaylist(cmd.Name)
		case 5:
			ok := GlobalPlayer.Reorder(cmd.Index, cmd.NewIndex)
			if !ok {
				// A general broadcast is sent on a successful change, an unsuccessful change triggers a send to just the offending client.
				s.SendTo(conn, ChangePlaylist, GlobalPlayer.State())
			}
		case 6:
			GlobalPlayer.JumpTo(cmd.Index)
		default:
			// Error.
			fmt.Println("Invalid Socket Message.")
		}
	}
}

type socketCmd struct {
	// 0: Change play state to "State" (<0 for echo state)
	// 1: Enqueue tracks listed in "IDs"
	// 2: Dequeue track given in "Index" (<0 for all)
	// 3: Load playlist given in "Name"
	// 4: Save current playlist as "Name"
	// 5: Move item at "Index" in the playlist to "NewIndex"
	Act int

	State    int    // Player Key for the desired action.
	IDs      []int  // List of track IDs from the DB.
	Index    int    // Playlist track index.
	NewIndex int    // Playlist track index.
	Name     string // Playlist name.
	AutoPlay bool   // If enqueuing items into an empty list should it start playing automatically?
}

type socketBroadcast struct {
	Change Change // 0: Play time, 1: Playing state, 2: Now playing, 3: Playlist, 4: Initial message, 5: Stream Description Change

	State *statusRes
}
