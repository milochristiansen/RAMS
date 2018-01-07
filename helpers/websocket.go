// +build ignore

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

package helpers

import "fmt"
import "sync"
import "net/http"

import "github.com/gorilla/websocket"

// Completely untested! I never even tried to compile this.
// Maybe later...

type WSCore struct {
	up *websocket.Upgrader

	clients map[*WSHandler]bool

	broadcast chan interface{}
	receive   chan interface{}

	reg   chan *WSHandler
	unreg chan *WSHandler

	mkmsg func() interface{}
}

func NewWSCore(mkmsg func() interface{}) *WSCore {
	core := &WSCore{
		up: &websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,

			CheckOrigin: func(r *http.Request) bool { return true },
		},

		broadcast: make(chan interface{}),
		receive:   make(chan interface{}),

		reg:   make(chan *WSHandler),
		unreg: make(chan *WSHandler),

		mkmsg: mkmsg,
	}

	go core.run()

	return core
}

func (core *WSCore) Receive() chan interface{} {
	return core.receive
}

func (core *WSCore) Broadcast(msg interface{}) {
	core.broadcast <- msg
}

func (core *WSCore) run() {
	clients := make(map[*WSHandler]bool)

	for {
		select {
		case handler := <-core.reg:
			clients[handler] = true
		case handler := <-core.unreg:
			if _, ok := clients[handler]; ok {
				delete(clients, handler)
				close(handler.broadcasts)
			}
		case msg := <-core.broadcast:
			for handler := range clients {
				select {
				case handler.broadcasts <- msg:
				default:
					close(handler.broadcasts)
					delete(clients, handler)
				}
			}
		}
	}
}

func (core *WSCore) UpgradeHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := core.up.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	handler := &WSHandler{core: core, conn: conn, broadcasts: make(chan interface{}, 5)}
	conn.reg <- handler

	go handler.in()
	go handler.out()
}

func (handler *WSHandler) in() {
	defer func() {
		handler.core.unreg <- handler
		handler.conn.Close()
	}()

	for {
		msg := handler.core.mkmsg()
		err := handler.conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				fmt.Println("Error:", err)
			}
			break
		}
		handler.core.receive <- msg
	}
}

func (handler *WSHandler) out() {
	defer handler.conn.Close()
	for {
		msg, ok := <-handler.broadcasts
		if !ok {
			handler.conn.WriteMessage(websocket.CloseMessage, []byte{})
			return
		}

		err := handler.conn.WriteJSON(msg)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
	}
}

type WSHandler struct {
	core       *WSCore
	conn       *websocket.Conn
	broadcasts chan interface{}
}
