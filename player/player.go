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
import "strings"
import "unsafe"
import "errors"
import "os"
import "encoding/json"

import "github.com/milochristiansen/RAMS/helpers"

//#include <stdlib.h>
//#include <vlc/vlc.h>
//
// extern void goEventCB(const struct libvlc_event_t*, void*);
//
// static int goAttach(libvlc_event_manager_t* em, libvlc_event_type_t et) {
// 	return libvlc_event_attach(em, et, goEventCB, NULL);
// }
import "C"

var GlobalPlayer *Player

type Player struct {
	sync.RWMutex

	evnts chan int
	keys  chan int

	root string
	db   string

	playlist   []int
	tracks     []*track
	playingIdx int

	stream string // Stream now-playing info.

	vlc *C.libvlc_instance_t
	mp  *C.libvlc_media_player_t
	em  *C.libvlc_event_manager_t
}

type track struct {
	media *C.libvlc_media_t // isStream == false
	path  string            // isStream == true

	isStream bool
}

func (tr *track) prepForPlay(vlc *C.libvlc_instance_t) {
	if tr.isStream {
		c := C.CString(tr.path)
		defer C.free(unsafe.Pointer(c))
		tr.media = C.libvlc_media_new_location(vlc, c)
	}
}
func (p *Player) restartStream() {
	tr := p.tracks[p.playingIdx]
	if tr.isStream {
		tr.prepForPlay(p.vlc)

		C.libvlc_media_player_set_media(p.mp, tr.media)
		C.libvlc_media_player_play(p.mp)

		sem := C.libvlc_media_event_manager(tr.media)
		C.goAttach(sem, C.libvlc_MediaMetaChanged)
	}
}

func InitPlayer(keys chan int, mediaRoot, dbServer string) {
	p := &Player{}
	p.evnts = make(chan int)
	p.root = mediaRoot
	p.db = dbServer
	p.keys = keys

	// TODO: Error check all the VLC functions.

	// TODO: Can I play from a memory buffer without too much trouble? It would be nice to preload tracks so disk/network latency isn't an issue.

	p.vlc = C.libvlc_new(0, nil)
	p.mp = C.libvlc_media_player_new(p.vlc)
	p.em = C.libvlc_media_player_event_manager(p.mp)

	// Don't forget the other places a new player is created!
	C.goAttach(p.em, C.libvlc_MediaPlayerEndReached)
	C.goAttach(p.em, C.libvlc_MediaPlayerTimeChanged)

	GlobalPlayer = p
	p = nil

	// Load what we were playing last time.
	GlobalPlayer.load()

	go func() {
		for {
			select {
			case key := <-GlobalPlayer.keys:
				switch key {
				case KeyNext:
					GlobalPlayer.changeTrack(true, true, true)
					GlobalSockets.Broadcast(ChangeNowPlaying, GlobalPlayer.State())
				case KeyPrev:
					GlobalPlayer.changeTrack(false, true, true)
					GlobalSockets.Broadcast(ChangeNowPlaying, GlobalPlayer.State())
				case KeyPause:
					GlobalPlayer.Lock()
					state := C.libvlc_media_player_get_state(GlobalPlayer.mp)
					if state != C.libvlc_Playing {
						GlobalPlayer.restartStream()
						C.libvlc_media_player_play(GlobalPlayer.mp)
					} else {
						C.libvlc_media_player_pause(GlobalPlayer.mp)
					}
					GlobalSockets.Broadcast(ChangeState, GlobalPlayer.state())
					GlobalPlayer.Unlock()
				case KeyStop:
					GlobalPlayer.Lock()
					C.libvlc_media_player_stop(GlobalPlayer.mp)
					GlobalSockets.Broadcast(ChangeState, GlobalPlayer.state())
					GlobalPlayer.Unlock()
				}
			case evnt := <-GlobalPlayer.evnts:
				switch evnt {
				case C.libvlc_MediaPlayerEndReached:
					GlobalPlayer.changeTrack(true, true, true)
					GlobalSockets.Broadcast(ChangeNowPlaying, GlobalPlayer.State())
				case C.libvlc_MediaPlayerTimeChanged:
					GlobalSockets.Broadcast(ChangeTime, GlobalPlayer.State())
				case C.libvlc_MediaMetaChanged:
					GlobalPlayer.RLock()
					tr := GlobalPlayer.tracks[GlobalPlayer.playingIdx]
					if tr.isStream {
						playingC := C.libvlc_media_get_meta(tr.media, C.libvlc_meta_NowPlaying)
						GlobalPlayer.stream = C.GoString(playingC)
						C.free(unsafe.Pointer(playingC))

						GlobalSockets.Broadcast(ChangeStream, GlobalPlayer.state())
					}
					GlobalPlayer.RUnlock()
				}
			}
		}
	}()
}

func (p *Player) FakeInput(key int) {
	switch {
	case key >= KeyNext || key <= KeyStop:
		// Pretend the user pressed the given key on the keyboard.
		p.keys <- key
	default:
		// Error
	}

}

func (p *Player) Reorder(idx, nidx int) bool {
	p.Lock()
	defer p.Unlock()

	if idx >= len(p.playlist) || nidx >= len(p.playlist) || idx < 0 || nidx < 0 {
		// Error
		return false
	}

	if idx == p.playingIdx {
		p.playingIdx = nidx
	}

	playlist, tracks := make([]int, len(p.playlist)), make([]*track, len(p.playlist))
	for i, j := 0, 0; i < len(p.playlist); i, j = i+1, j+1 {
		switch {
		case i == idx:
			playlist[nidx], tracks[nidx] = p.playlist[i], p.tracks[i]
			j--
		case j == nidx:
			j++
			fallthrough
		default:
			playlist[j], tracks[j] = p.playlist[i], p.tracks[i]
		}
	}
	p.playlist, p.tracks = playlist, tracks

	GlobalSockets.Broadcast(ChangePlaylist, p.state())
	return true
}

type dbTrack struct {
	ID   int
	Path string

	// For logging
	Name   string
	Artist string

	// Lots of other fields we do not care about
}

func (p *Player) JumpTo(idx int) {
	if idx >= len(p.playlist) || idx < 0 {
		return
	}

	p.Lock()
	defer p.Unlock()

	p.playingIdx = idx
	C.libvlc_media_player_set_media(p.mp, p.tracks[idx].media)
	C.libvlc_media_player_play(p.mp)
	p.save()

	GlobalSockets.Broadcast(ChangeNowPlaying, p.state())
}

func (p *Player) changeTrack(next, lock, save bool) {
	if len(p.playlist) == 0 {
		return
	}

	if lock {
		p.Lock()
		defer p.Unlock()
	}

	nextIdx := p.playingIdx
	if next {
		nextIdx++
	} else {
		nextIdx--
	}
	if nextIdx < 0 {
		nextIdx = len(p.playlist) - 1
	}
	if nextIdx >= len(p.playlist) {
		nextIdx = 0
	}

	tr := p.tracks[nextIdx]
	tr.prepForPlay(p.vlc)

	C.libvlc_media_player_set_media(p.mp, tr.media)
	C.libvlc_media_player_play(p.mp)
	p.playingIdx = nextIdx

	if tr.isStream {
		sem := C.libvlc_media_event_manager(tr.media)
		C.goAttach(sem, C.libvlc_MediaMetaChanged)
	}

	if save {
		p.save()
	}
}

// SavePlaylist saves the current player state to a dump file.
func (p *Player) SavePlaylist(name string) error {
	f, err := os.Create("./" + name + ".json")
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.Encode(p.state())
	return nil
}

func (p *Player) save() {
	err := p.SavePlaylist("nowplaying")
	if err != nil {
		// TODO: Handle properly!
		panic(err)
	}
}

// LoadPlaylist loads the given state dump file, optionally continuing from the last played track.
func (p *Player) LoadPlaylist(name string, cont bool) error {
	f, err := os.Open("./" + name + ".json")
	if err != nil {
		return errors.New("Could not load playlist: " + name)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	np := &statusRes{}
	err = dec.Decode(&np)
	if err != nil {
		return errors.New("Could not load playlist: " + name)
	}

	p.Lock()
	defer p.Unlock()

	// Dequeue
	C.libvlc_media_player_stop(p.mp)

	for _, m := range p.tracks {
		C.libvlc_media_release(m.media)
	}

	p.playlist = np.Playlist
	p.tracks = nil
	p.playingIdx = np.Playing - 1
	if !cont {
		p.playingIdx = -1
	}

	// Enqueue
	r, err := helpers.JSONQuery(p.db+"/data/", p.playlist, func() interface{} { return &[]*dbTrack{} })
	if err != nil {
		return err
	}
	trackData := *r.(*[]*dbTrack)

	if len(trackData) != len(p.playlist) {
		// Sanity check
		panic("DB returned invalid data!")
	}

	for i := range p.playlist {
		t := loadMedia(p.vlc, trackData[i].Path, p.root)
		if t == nil {
			fmt.Println("Could not load given media: " + trackData[i].Path)
			continue
		}

		p.tracks = append(p.tracks, t)
	}

	p.changeTrack(true, false, false)
	GlobalSockets.Broadcast(ChangePlaylist, p.state())
	GlobalSockets.Broadcast(ChangeNowPlaying, p.state())
	return nil
}

func (p *Player) load() {
	err := p.LoadPlaylist("nowplaying", true)
	if err != nil {
		return
	}
}

// Enqueue the track(s) with the given ID(s) in the playlist.
func (p *Player) Enqueue(autoplay bool, ids ...int) {
	p.Lock()
	defer p.Unlock()

	r, err := helpers.JSONQuery(p.db+"/data/", ids, func() interface{} { return &[]*dbTrack{} })
	if err != nil {
		// TODO: Handle properly!
		panic(err)
	}
	trackData := *r.(*[]*dbTrack)

	if len(trackData) != len(ids) {
		// Sanity check
		panic("DB returned invalid data!")
	}

	oc := len(p.playlist)

	for i, id := range ids {
		t := loadMedia(p.vlc, trackData[i].Path, p.root)
		if t == nil {
			fmt.Println("Could not load given media: " + trackData[i].Path)
			continue
		}

		p.playlist = append(p.playlist, id)
		p.tracks = append(p.tracks, t)
	}

	if oc == 0 && autoplay {
		p.playingIdx = len(p.playlist) - 1
		p.changeTrack(true, false, true)
		GlobalSockets.Broadcast(ChangeNowPlaying, p.state())
	} else {
		p.save()
	}
	GlobalSockets.Broadcast(ChangePlaylist, p.state())
}

// Dequeue the track with the given playlist index. If index is negative then clear the whole playlist.
func (p *Player) Dequeue(index int) {
	p.Lock()
	defer p.Unlock()

	// Clear all:
	if index < 0 {
		C.libvlc_media_player_stop(p.mp)
		C.libvlc_media_player_release(p.mp)
		p.mp = C.libvlc_media_player_new(p.vlc)
		p.em = C.libvlc_media_player_event_manager(p.mp)
		C.goAttach(p.em, C.libvlc_MediaPlayerEndReached)
		C.goAttach(p.em, C.libvlc_MediaPlayerTimeChanged)

		for _, m := range p.tracks {
			C.libvlc_media_release(m.media)
		}

		p.playlist = nil
		p.tracks = nil
		p.playingIdx = 0
		p.save()
		GlobalSockets.Broadcast(ChangePlaylist, p.state())
		GlobalSockets.Broadcast(ChangeNowPlaying, p.state())
		return
	}

	// Remove 1:
	if index >= len(p.playlist) || index < 0 {
		return
	}

	if len(p.playlist) == 1 {
		C.libvlc_media_player_stop(p.mp)
		C.libvlc_media_player_release(p.mp)
		p.mp = C.libvlc_media_player_new(p.vlc)
		p.em = C.libvlc_media_player_event_manager(p.mp)
		C.goAttach(p.em, C.libvlc_MediaPlayerEndReached)
		C.goAttach(p.em, C.libvlc_MediaPlayerTimeChanged)
	} else if index == p.playingIdx {
		p.changeTrack(true, false, false)
		GlobalSockets.Broadcast(ChangeNowPlaying, p.state())
	}

	C.libvlc_media_release(p.tracks[index].media)
	p.tracks = append(p.tracks[:index], p.tracks[index+1:]...)
	p.playlist = append(p.playlist[:index], p.playlist[index+1:]...)

	if index < p.playingIdx {
		p.playingIdx--
	}
	p.save()
	GlobalSockets.Broadcast(ChangePlaylist, p.state())
}

// Read the player state.
func (p *Player) State() *statusRes {
	p.RLock()
	defer p.RUnlock()

	return p.state()
}

func (p *Player) state() *statusRes {
	state := C.libvlc_media_player_get_state(p.mp)
	rstate := 2
	switch state {
	case C.libvlc_Paused:
		rstate = 1
	case C.libvlc_Stopped:
		fallthrough
	case C.libvlc_Ended:
		fallthrough
	case C.libvlc_Error:
		rstate = 0
	}

	isstream := false
	if p.playingIdx >= 0 && p.playingIdx < len(p.tracks) {
		isstream = p.tracks[p.playingIdx].isStream
	}

	stream := ""
	if isstream {
		stream = p.stream
	}

	playlist := p.playlist
	if playlist == nil {
		playlist = []int{}
	}

	return &statusRes{
		Playlist: playlist,
		Playing:  p.playingIdx,

		Elapsed:  int(C.libvlc_media_player_get_time(p.mp)),
		Duration: int(C.libvlc_media_player_get_length(p.mp)),

		StreamInfo: stream,
		IsStream:   isstream,

		State: rstate,
	}
}

type statusRes struct {
	Playlist []int // Tracks IDs of the songs in the playlist
	Playing  int   // Index into playlist, NOT a track ID

	// For current track, not whole playlist
	Elapsed  int
	Duration int

	StreamInfo string
	IsStream   bool

	State int // 0: Stopped, 1: Paused, 2: Playing
}

func loadMedia(vlc *C.libvlc_instance_t, path, root string) *track {
	if strings.HasPrefix(path, "media/") {
		// Is "local" file (AXIS path).
		path = ReplacePrefix(path, "media/", root)

		c := C.CString(path)
		defer C.free(unsafe.Pointer(c))

		m := C.libvlc_media_new_path(vlc, c)
		if m == nil {
			return nil
		}

		return &track{m, "", false}
	}

	// Is URL. May not be a stream, but how can you tell?
	return &track{nil, path, true}
}

func ReplacePrefix(s, prefix, replace string) string {
	if strings.HasPrefix(s, prefix) {
		s = s[len(prefix):]
		return replace + s
	}
	return s
}
