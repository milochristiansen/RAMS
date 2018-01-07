
// Order matters! This way max can be less than min, and things still work.
function clamp(i, min, max) {
	return Math.max(Math.min(i, max), min)
}

var Core = null
var Conn = null

class hyperlist {
	constructor(id, listyp, sort, dir, singlesel, factory){
		this.id = id
		this.factory = factory

		this.listyp = listyp
		this.sort = sort
		this.asc = dir == "asc"

		this.items = []
		this.data = []
		this.inflight = []
		this.waiting = []

		this.filter = null
		this.onselect = null

		// List of all selected items
		this.selected = singlesel ? null : {}
		this.selectedorder = singlesel ? null : []
		this.singlesel = singlesel

		// Grab a null element to calculate height.
		var el = $(this.makerow(null)).hide()
		var container = $(`#${id}`)
		container.append(el)
		var height = el.height()
		container.empty()
		
		var host = this
		this.config = {
			itemHeight: height,
			total: 0,

			generate(idx) {
				return host.makerow(idx)
			},
		}

		// Future home of the hyperlist object.
		this.list = null
	}

	refresh(){
		var req = new XMLHttpRequest()
		var host = this
		req.onload = function(){
			if (this.status == 200) {
				host.items = JSON.parse(this.responseText)
				host.data = []
				host.inflight = []
				host.waiting = []
				host.config.total = host.items.length

				var st = host.fixselection()

				if (host.list == null) {
					host.list = HyperList.create($(`#${host.id}`)[0], host.config)
				} else {
					host.list.refresh($(`#${host.id}`)[0], host.config)
				}
				host.scrollTo(st)
			} else {
				console.log("Response error: "+this.status)
			}
		}
		req.open("POST", Core.settings.DB+"/list")
		var extra = ""
		if (this.filter) {
			extra = this.filter()
		}
		req.send(`{"typ": "${this.listyp}",${extra} "sort": "${this.sort}", "asc": ${this.asc}}`)
	}

	resort(sort){
		this.asc = (sort === this.sort) ? !this.asc : this.asc
		this.sort = sort
		this.refresh()
	}

	scrollTo(idx){
		if (idx < 0) {
			// Incorrect, but the browser clamps the value automatically.
			this.list._element.scrollTop = this.list._element.scrollHeight
			return
		}
		// Once again, don't worry about range the browser clamps this.
		this.list._element.scrollTop = this.config.itemHeight * idx
	}

	select(idx){
		var item = this.listyp === "track" ? this.data[idx] : this.items[idx] 
		if (item === undefined) {
			console.log("Impossible selection.")
			return
		}

		if (this.singlesel) {
			var issel = $(`#${this.listyp+idx}`).hasClass("selected")
			$(`.${this.listyp}`).removeClass("selected")
			if (!issel) {
				$(`#${this.listyp+idx}`).addClass("selected")
			}
			this.selected = issel ? null : (this.listyp === "track" ? item.ID : item.Name)
			this.selectedorder = issel ? null : idx
		} else {
			$(`#${this.listyp+idx}`).toggleClass("selected")

			if ($(`#${this.listyp+idx}`).hasClass("selected")) {
				this.selected[(this.listyp === "track" ? item.ID : item.Name)] = true
				this.selectedorder[idx] = (this.listyp === "track" ? item.ID : item.Name)
			} else {
				delete this.selected[(this.listyp === "track" ? item.ID : item.Name)]
				this.selectedorder[idx] = null
			}
		}

		if (this.onselect != null) {
			this.onselect()
		}
	}

	clearselect(){
		$(`.${this.listyp}`).removeClass("selected")
		if (this.singlesel) {
			this.selected = null
			this.selectedorder = null
		} else {
			this.selected = {}
			this.selectedorder = []
		}
	}

	isselected(idx){
		return this.singlesel ? this.selectedorder === idx : this.selectedorder[idx] != null 
	}

	fixselection(){
		if (this.singlesel) {
			for (var i = 0; i < this.items.length; i++) {
				var item = this.items[i]
				if (this.selected === (this.listyp === "track" ? item : item.Name)) {
					this.selectedorder = i
					return i
				}
			}
			this.selected = null
			this.selectedorder = null
			return 0
		}
		var selected = {}
		var selectedorder = []
		var first = -1
		for (var i = 0; i < this.items.length; i++) {
			var item = this.items[i]
			var iid = (this.listyp === "track" ? item : item.Name)
			if (iid in this.selected) {
				selected[iid] = true
				selectedorder[i] = iid
				first = first == -1 ? i : first
			}
		}
		this.selected = selected
		this.selectedorder = selectedorder
		return first >= 0 ? first : 0
	}

	makerowhelper(idx, text){
		return $(`<div onclick="Core.listSelect('${this.listyp}', ${idx})"${this.listyp == "track" ? ' ondblclick="Core.enqueueAndPlay('+idx+')"' : ""}
			id="${this.listyp+idx}"
			class="list-row ${this.listyp}${this.isselected(idx) ? " selected" : ""}">${text}</div>`)[0]
	}

	makerow(idx){
		if (idx == null) {
			return $(`<div class="list-row ${this.listyp}">Height Check!</div>`)[0]
		}
		if (this.inflight[idx] || this.waiting[idx]) {
			this.waiting[idx] = true
			return this.makerowhelper(idx, "Loading...")
		}

		if (this.listyp !== "track") {
			return this.makerowhelper(idx, this.factory(this.items[idx]))
		}

		if (this.data[idx] !== undefined) {
			return this.makerowhelper(idx, this.factory(this.data[idx]))
		}

		// For tracks we need to do a request for the track data.
		// To help performance as much as possible we do requests in blocks.

		var f = idx-40
		var l = idx+40
		if (f < 0) {
			f = 0
		}
		if (l > this.items.length) {
			l = this.items.length
		}
		var items = []
		for (var i = f; i < l; i++) {
			this.inflight[i] = true
			items.push(this.items[i])
		}

		var req = new XMLHttpRequest()
		var host = this
		req.onload = function(){
			if (this.status == 200) {
				var rows = JSON.parse(this.responseText)
				for (var i = 0; i < rows.length; i++) {
					if (items[i] === undefined) {
						console.log("Malformed response.")
						return
					}
					
					if (host.waiting[i+f]) {
						host.waiting[i+f] = false
						// When scrolling *really* fast elements can disappear before the DB responds.
						// This happens most often when scrolling to a specific item via scrollTo after a refresh.
						var el = $(`#${host.listyp+(i+f)}`).get(0)
						if (el != undefined){
							el.innerHTML = host.factory(rows[i])
						}
					}
					host.inflight[i+f] = false
					host.data[i+f] = rows[i]
				}
			} else {
				var es = "Error: "+this.status
				for (var i = 0; i < items.length; i++) {
					$(`#${host.listyp+(i+f)}`)[0].innerHTML = es
				}
				console.log("Response error: "+this.status)
			}
		}
		req.onerror = function(){
			for (var i = 0; i < items.length; i++) {
				$(`#${host.listyp+(i+f)}`)[0].innerHTML = "Error."
			}
			console.log("Could not contact server.")
		}
		req.open("POST", Core.settings.DB+"/data/")
		req.send(JSON.stringify(items))

		this.waiting[idx] = true
		return this.makerowhelper(idx, "Loading...")
	}
}

class playlist {
	constructor() {
		this.pdata = {Playlist: []} // pdata.Playlist is the list of IDs, the other keys are player state info.
		this.data = []
	}

	scrollTo(idx){
		var el = $(`.plbody div:nth-of-type(${idx})`).get(0)
		if (el != undefined) {
			el.scrollIntoView(true)
		}
	}

	refresh(){
		if (!("Playlist" in this.pdata) || this.pdata.Playlist.length == 0) {
			this.data = []
			return
		}
		
		var itemwindow = JSON.stringify(this.pdata.Playlist)
		var req = new XMLHttpRequest()
		var host = this
		req.onload = function(){
			if (this.status == 200) {
				host.data = JSON.parse(this.responseText)
			} else {
				host.data = []
				console.log("Response error: "+this.status)
			}
		}
		req.onerror = function(){
			host.fillwindowErr()
			console.log("Could not contact server.")
		}
		req.open("POST", Core.settings.DB+"/data/")
		req.send(`${itemwindow}`)
	}
}

$(document).ready(function() {
	Core = new Vue({
		el: "#Main",
		data: {
			artists: new hyperlist("ArtistList", "artist", "name", "desc", true, function(v){
				return `<span class="fill"  >${(v.Name !== undefined) ? v.Name : "-"}</span>
						<span class="number">${(v.Albums !== undefined) ? v.Albums : "-"}</span>
						<span class="number">${(v.Tracks !== undefined) ? v.Tracks : "-"}</span>`
			}),
			albums: new hyperlist("AlbumList", "album", "year", "desc", true, function(v){
				return `<span class="fill"  >${(v.Name !== undefined) ? v.Name : "-"}</span>
						<span class="number">${(v.Tracks !== undefined) ? v.Tracks : "-"}</span>
						<span class="year"  >${(v.Year !== undefined) ? (v.Year !== 0 ? v.Year : "----") : "----"}</span>`
			}),
			tracks: new hyperlist("TrackList", "track", "album", "asc", false, function(v){
				return `<span class="button" onclick="Core.openEditBox(${v.ID})">#</span>
						<span class="artist">${(v.Artist !== undefined) ? v.Artist : "-"}</span>
						<span class="album" >${(v.Album !== undefined) ? v.Album : "-"}</span>
						<span class="number">${(v.Disk !== undefined) ? v.Disk : "-"}</span>
						<span class="number">${(v.Track !== undefined) ? v.Track : "-"}</span>
						<span class="name"  >${(v.Name !== undefined) ? v.Name : "-"}</span>
						<span class="time"  >${(v.Length !== undefined) ? Core.minutes(v.Length)+":"+Core.seconds(v.Length) : "-"}</span>
						<span class="year"  >${(v.Year !== undefined) ? (v.Year !== 0 ? v.Year : "----") : "----"}</span>
						<span class="fill"  >${(v.Path !== undefined) ? v.Path : "-"}</span>`
			}),
			playlist: new playlist(),

			dragging: 0,

			editRow: {},
			editShow: false,
			editPos: {
				top: (window.innerHeight/2-window.innerHeight/4)+"px", left: (window.innerWidth/3)+"px",
				x: window.innerWidth/3, y: window.innerHeight/2-window.innerHeight/4,
			},
			
			settings: {
				DB: "http://127.0.0.1:2330",
				Player: "ws://127.0.0.1:2331",
			},
			settingsShow: false,
			settingsPos: {
				top: (window.innerHeight/2-window.innerHeight/4)+"px", left: (window.innerWidth/3)+"px",
				x: window.innerWidth/3, y: window.innerHeight/2-window.innerHeight/4,
			},

			nowPlaying: {
				thumb: {
					left: $(`.now-playing .thumb`).position().left+"px",
				},
				albumArt: "",
				trackDesc: "",
				state: {},
			},
		},
		methods: {
			begindrag: function(id){
				this.dragging = id
			},
			drag: function(evnt){
				// TODO: Works, but rather poorly.
				switch (this.dragging) {
				case 0:
					return
				case 1:
					this.editPos.x += evnt.movementX
					this.editPos.y += evnt.movementY
					this.editPos.left = this.editPos.x+"px"
					this.editPos.top = this.editPos.y+"px"
					return
				case 2:
					this.settingsPos.x += evnt.movementX
					this.settingsPos.y += evnt.movementY
					this.settingsPos.left = this.settingsPos.x+"px"
					this.settingsPos.top = this.settingsPos.y+"px"
					return
				}
			},
			dragend: function(){
				this.dragging = 0
			},

			openEditBox: function(id){
				var req = new XMLHttpRequest()
				var host = this
				req.onload = function(){
					if (this.status == 200) {
						host.editRow = JSON.parse(this.responseText)[0]
						host.editShow = true
					} else {
						console.log("Response error: "+this.status)
					}
				}
				req.onerror = function(){
					console.log("Could not contact server.")
				}
				req.open("GET", this.settings.DB+"/data/"+id)
				req.send()
			},
			saveEditBox: function(){
				var req = new XMLHttpRequest()
				var host = this
				req.onload = function(){
					if (this.status == 200) {
						// TODO: It would be best if I updated only the effected row.
						this.tracks.refresh()
					} else {
						console.log("Response error: "+this.status)
					}
				}
				req.onerror = function(){
					console.log("Could not contact server.")
				}
				req.open("POST", this.settings.DB+"/update")
				req.send(`${JSON.stringify(host.editRow)}`)

				this.closeEditBox()
			},
			closeEditBox: function(){
				this.editRow = {}
				this.editShow = false
			},

			saveSettingsBox: function(){
				// Save settings for later.
				window.localStorage["RAMS.PlayerUI.Settings"] = JSON.stringify(this.settings)

				this.closeSettingsBox()
			},
			closeSettingsBox: function(){
				this.settingsShow = false
			},

			listSelect: function(list, idx){
				switch (list) {
				case "artist":
					this.artists.select(idx)
					break
				case "album":
					this.albums.select(idx)
					break
				case "track":
					this.tracks.select(idx)
					break
				}
			},

			enqueuePL: function(){
				var items = []
				for (var i = 0; i < this.tracks.selectedorder.length; i++) {
					id = this.tracks.selectedorder[i]
					if (typeof id == "number") {
						items.push(id)
					}
				}
				this.tracks.clearselect()

				Conn.send(`{"Act": 1, "IDs": ${JSON.stringify(items)}}`)
			},
			enqueuePLAll: function(album){
				if (album && this.albums.selected == null) {
					return
				}

				if (this.tracks.items.length > 500) {
					alert("Too many tracks selected. Try queuing a few at a time.")
					return
				} else if (this.tracks.items.length > 100) {
					alert("WARNING: Queuing large numbers of tracks at once may take some time.")
				}

				var items = []
				for (var i = 0; i < this.tracks.items.length; i++) {
					items.push(this.tracks.items[i])
				}

				Conn.send(`{"Act": 1, "IDs": ${JSON.stringify(items)}}`)
			},
			enqueueAndPlay: function(idx){
				if (this.tracks.items.length > 500) {
					alert("Too many tracks selected. Try queuing a few at a time.")
					return
				} else if (this.tracks.items.length > 100) {
					alert("WARNING: Queuing large numbers of tracks at once may take some time.")
				}

				Conn.send(`{"Act": 2, "Index": -1}`)
				Conn.send(`{"Act": 1, "IDs": ${JSON.stringify(this.tracks.items)}, "AutoPlay": false}`)
				Conn.send(`{"Act": 6, "Index": ${idx}}`)
			},
			jumpToPLItem: function(idx){
				Conn.send(`{"Act": 6, "Index": ${idx}}`)
			},
			resortPLItem: function(evnt){
				Conn.send(`{"Act": 5, "Index": ${evnt.oldIndex}, "NewIndex": ${evnt.newIndex}}`)
			},
			deletePLItem: function(evnt){
				$("#PLDeleter")[0].innerHTML = "" // Goodbye.
				Conn.send(`{"Act": 2, "Index": ${evnt.oldIndex}}`)
			},
			clearplaylist: function(){
				Conn.send(`{"Act": 2, "Index": -1}`)
			},

			updatePlayProgress: function(){
				var gutter = $(`.now-playing .gutter`)
				var thumb = $(`.now-playing .thumb`)
				var left = gutter.position().left
				var range = gutter.width() - thumb.outerWidth(true)

				var percent = 0
				if (this.nowPlaying.state.Duration > 0) {
					var percent = this.nowPlaying.state.Elapsed / this.nowPlaying.state.Duration
				}

				this.nowPlaying.thumb.left = left+(range*percent)+"px"
			},

			playercommand: function(command){
				switch (command) {
				case "prev":
					command = "2"
					break
				case "next":
					command = "1"
					break
				case "play":
					command = "3"
					break
				case "stop":
					command = "4"
					break
				default:
					command = "0"
					break
				}
				Conn.send(`{"Act": 0, "State": ${command}}`)
			},

			seconds: function(n) {
				if (isNaN(n) || n < 0) {
					n = 0
				}
				n = Math.floor(n / 1000 % 60)
				n = n+""
				return ("00"+n).substring(n.length)
			},

			minutes: function(n) {
				if (isNaN(n) || n < 0) {
					n = 0
				}
				n = Math.floor(n / 60000)
				n = n+""
				return ("00"+n).substring(n.length)
			},
		},
	})
	Core.artists.filter = function(){
		return ` "album": ${Core.albums.selected != null ? JSON.stringify(Core.albums.selected) : '""'},`
	}
	Core.albums.filter = function(){
		return ` "artist": ${Core.artists.selected != null ? JSON.stringify(Core.artists.selected) : '""'},`
	}
	Core.tracks.filter = function(){
		return ` "artist": ${Core.artists.selected != null ? JSON.stringify(Core.artists.selected) : '""'}, "album": ${Core.albums.selected != null ? JSON.stringify(Core.albums.selected) : '""'},`
	}

	Core.artists.onselect = function(){
		Core.albums.refresh()
		Core.tracks.refresh()
	}
	Core.albums.onselect = function(){
		Core.artists.refresh()
		Core.tracks.refresh()
	}

	var savedata = window.localStorage["RAMS.PlayerUI.Settings"]
	if (savedata) {
		var settings = JSON.parse(savedata)

		Core.settings.DB     = settings.DB     !== undefined ? settings.DB     : Core.settings.DB
		Core.settings.Player = settings.Player !== undefined ? settings.Player : Core.settings.Player

		Core.saveSettingsBox()
	}

	// Delay initialization of content until here. Settings need to be ready before it will work.
	Core.artists.refresh()
	Core.albums.refresh()
	Core.tracks.refresh()
	Core.updatePlayProgress()

	var initDone = true
	Conn = new ReconnectingWebSocket(Core.settings.Player+"/socket")
	Conn.onmessage = function(evnt) {
		var msg = JSON.parse(evnt.data)
		switch (msg.Change) {
		case 4:
			// Every new connection gets this message as the very first thing sent.
			// In this case everything should refresh.
			initDone = false
			// Fallthrough
		case 0: // Time
			// Fallthrough
		case 1: // Play state
			Core.nowPlaying.state = msg.State
			Core.updatePlayProgress()
			if (initDone) { break }
		case 5: // Stream now playing changed.
		case 2: // New track
			Core.nowPlaying.state = msg.State
			Core.updatePlayProgress()
			if (msg.State.Playing >= 0) {
				Core.playlist.scrollTo(msg.State.Playing)
			}

			var playing = msg.State.Playlist[msg.State.Playing]
			if (playing !== undefined && !msg.State.IsStream) {
				var req = new XMLHttpRequest()
				var host = this
				req.onload = function(){
					if (this.status == 200) {
						var row = JSON.parse(this.responseText)
						
						Core.nowPlaying.trackDesc = `${row[0].Artist} - ${row[0].Name}`
						if ("Notification" in window) {
							if (Notification.permission !== "granted") {
								Notification.requestPermission()
							}
							new Notification(`RAMS: New Track`, {
								icon: `${Core.settings.DB}/art/${playing}`,
								body: Core.nowPlaying.trackDesc
							})
						}

						Core.nowPlaying.albumArt = `${Core.settings.DB}/art/${playing}`
					} else {
						console.log("Response error: "+this.status)
					}
				}
				req.onerror = function(){
					console.log("Could not contact server.")
				}
				req.open("GET", Core.settings.DB+"/data/"+playing)
				req.send()
			} else if (msg.State.IsStream && msg.State.StreamInfo != "") {
				if ("Notification" in window) {
					if (Notification.permission !== "granted") {
						Notification.requestPermission()
					}
					new Notification(`RAMS: New Track`, {
						icon: `${Core.settings.DB}/art/${playing}`,
						body: `${msg.State.StreamInfo}`
					})
				}
				Core.nowPlaying.trackDesc = msg.State.StreamInfo
				Core.nowPlaying.albumArt = `${Core.settings.DB}/art/${playing}`
			} else {
				// If the playlist is empty or playing index is invalid.
				Core.nowPlaying.albumArt = `${Core.settings.DB}/art/-1`
				Core.playlist.scrollTo(0)
			}
			// DO NOT BREAK
			// New track should fallthrough so that the playlist position updates.
		case 3: // Playlist
			Core.playlist.pdata = msg.State
			Core.playlist.refresh()
			if (initDone) { break }
		case 5: // Stream now playing changed.
		}
		initDone = true
	}
})
