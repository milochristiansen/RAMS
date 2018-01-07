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

package db

import "sync"
import "sort"
import "strings"

type sortOrder int

// Three lists for artists:
//	* By name
//	* By album count
//	* By track count
// Three lists for albums:
//	* By name
//	* By track count
//	* By year
// Plus
//	* Yet another album list triplet for each artist and vice versa

func ListArtists(album string, sort sortOrder, asc bool) []ArtistData {
	globalListCache.RLock()
	defer globalListCache.RUnlock()

	ald, ok := globalListCache.albums[album]
	if !ok {
		switch sort {
		case SortTracks:
			if asc {
				return copyArtistList(globalListCache.artistsByTracksRev)
			} else {
				return copyArtistList(globalListCache.artistsByTracks)
			}
		case SortAlbums:
			if asc {
				return copyArtistList(globalListCache.artistsByAlbumsRev)
			} else {
				return copyArtistList(globalListCache.artistsByAlbums)
			}
		case SortName:
			fallthrough
		default:
			if asc {
				return copyArtistList(globalListCache.artistsByNameRev)
			} else {
				return copyArtistList(globalListCache.artistsByName)
			}
		}
	}

	switch sort {
	case SortTracks:
		if asc {
			return copyArtistList(ald.artistsByTracksRev)
		} else {
			return copyArtistList(ald.artistsByTracks)
		}
	case SortAlbums:
		if asc {
			return copyArtistList(ald.artistsByAlbumsRev)
		} else {
			return copyArtistList(ald.artistsByAlbums)
		}
	case SortName:
		fallthrough
	default:
		if asc {
			return copyArtistList(ald.artistsByNameRev)
		} else {
			return copyArtistList(ald.artistsByName)
		}
	}
}

func ListAlbums(artist string, sort sortOrder, asc bool) []AlbumData {
	globalListCache.RLock()
	defer globalListCache.RUnlock()

	ard, ok := globalListCache.artists[artist]
	if !ok {
		switch sort {
		case SortTracks:
			if asc {
				return copyAlbumList(globalListCache.albumsByTracksRev)
			} else {
				return copyAlbumList(globalListCache.albumsByTracks)
			}
		case SortYear:
			if asc {
				return copyAlbumList(globalListCache.albumsByYearRev)
			} else {
				return copyAlbumList(globalListCache.albumsByYear)
			}
		case SortName:
			fallthrough
		default:
			if asc {
				return copyAlbumList(globalListCache.albumsByNameRev)
			} else {
				return copyAlbumList(globalListCache.albumsByName)
			}
		}
	}

	switch sort {
	case SortTracks:
		if asc {
			return copyAlbumList(ard.albumsByTracksRev)
		} else {
			return copyAlbumList(ard.albumsByTracks)
		}
	case SortYear:
		if asc {
			return copyAlbumList(ard.albumsByYearRev)
		} else {
			return copyAlbumList(ard.albumsByYear)
		}
	case SortName:
		fallthrough
	default:
		if asc {
			return copyAlbumList(ard.albumsByNameRev)
		} else {
			return copyAlbumList(ard.albumsByName)
		}
	}
}

type ArtistData struct {
	Name   string
	Tracks int
	Albums int
}

type AlbumData struct {
	Name   string
	Tracks int
	Year   int
}

// Call whenever the DB changes
func RefreshLists() error {
	rows, err := listCacheQuery.Query()
	if err != nil {
		return err
	}
	defer rows.Close()

	tracks := make([]*listCacheTrack, 0, 0x3fff)
	for rows.Next() {
		t := &listCacheTrack{}
		err := rows.Scan(&t.Artist, &t.Album, &t.Year)
		if err != nil {
			return err
		}
		tracks = append(tracks, t)
	}

	globalListCache.Lock()
	defer globalListCache.Unlock()

	// Make sure there is no stale data.
	globalListCache.artists = map[string]*listCacheArtist{}

	globalListCache.artistsByName = nil
	globalListCache.artistsByTracks = nil
	globalListCache.artistsByAlbums = nil

	globalListCache.artistsByNameRev = nil
	globalListCache.artistsByTracksRev = nil
	globalListCache.artistsByAlbumsRev = nil

	globalListCache.albums = map[string]*listCacheAlbum{}

	globalListCache.albumsByName = nil
	globalListCache.albumsByTracks = nil
	globalListCache.albumsByYear = nil

	globalListCache.albumsByNameRev = nil
	globalListCache.albumsByTracksRev = nil
	globalListCache.albumsByYearRev = nil

	// Populate the master artist list

	for _, t := range tracks {
		if t.Artist != "" {
			ard, ok := globalListCache.artists[t.Artist]
			if !ok {
				ard = &listCacheArtist{
					data:   &ArtistData{t.Artist, 0, 0},
					albums: map[string]*AlbumData{},
				}
				globalListCache.artists[t.Artist] = ard

				globalListCache.artistsByName = append(globalListCache.artistsByName, ard.data)
				globalListCache.artistsByTracks = append(globalListCache.artistsByTracks, ard.data)
				globalListCache.artistsByAlbums = append(globalListCache.artistsByAlbums, ard.data)

				globalListCache.artistsByNameRev = append(globalListCache.artistsByNameRev, ard.data)
				globalListCache.artistsByTracksRev = append(globalListCache.artistsByTracksRev, ard.data)
				globalListCache.artistsByAlbumsRev = append(globalListCache.artistsByAlbumsRev, ard.data)
			}

			ard.data.Tracks++
		}

		// OK, the artist data is done, now for the album:

		if t.Album != "" {
			ald, ok := globalListCache.albums[t.Album]
			if !ok {
				ald = &listCacheAlbum{
					data:    &AlbumData{t.Album, 0, 0},
					artists: map[string]*ArtistData{},
				}

				globalListCache.albums[t.Album] = ald

				globalListCache.albumsByName = append(globalListCache.albumsByName, ald.data)
				globalListCache.albumsByTracks = append(globalListCache.albumsByTracks, ald.data)
				globalListCache.albumsByYear = append(globalListCache.albumsByYear, ald.data)

				globalListCache.albumsByNameRev = append(globalListCache.albumsByNameRev, ald.data)
				globalListCache.albumsByTracksRev = append(globalListCache.albumsByTracksRev, ald.data)
				globalListCache.albumsByYearRev = append(globalListCache.albumsByYearRev, ald.data)
			}
			ald.data.Tracks++
			if t.Year > ald.data.Year {
				ald.data.Year = t.Year
			}
		}

		// Now for the individual lists

		if t.Album != "" && t.Artist != "" {
			// Artist album list
			ard, ok := globalListCache.artists[t.Artist]
			if !ok {
				panic("Combobulater discombobulated.")
			}

			arald, ok := ard.albums[t.Album]
			if !ok {
				arald = &AlbumData{t.Album, 0, 0}

				ard.data.Albums++

				ard.albums[t.Album] = arald

				ard.albumsByName = append(ard.albumsByName, arald)
				ard.albumsByTracks = append(ard.albumsByTracks, arald)
				ard.albumsByYear = append(ard.albumsByYear, arald)

				ard.albumsByNameRev = append(ard.albumsByNameRev, arald)
				ard.albumsByTracksRev = append(ard.albumsByTracksRev, arald)
				ard.albumsByYearRev = append(ard.albumsByYearRev, arald)
			}
			arald.Tracks++
			if t.Year > arald.Year {
				arald.Year = t.Year
			}

			// Album artist list
			ald, ok := globalListCache.albums[t.Album]
			if !ok {
				panic("Combobulater discombobulated.")
			}

			alard, ok := ald.artists[t.Artist]
			if !ok {
				alard = &ArtistData{t.Artist, 0, 0}
				alard.Albums = 1

				ald.artists[t.Artist] = alard

				ald.artistsByName = append(ald.artistsByName, alard)
				ald.artistsByTracks = append(ald.artistsByTracks, alard)
				ald.artistsByAlbums = append(ald.artistsByAlbums, alard)

				ald.artistsByNameRev = append(ald.artistsByNameRev, alard)
				ald.artistsByTracksRev = append(ald.artistsByTracksRev, alard)
				ald.artistsByAlbumsRev = append(ald.artistsByAlbumsRev, alard)
			}
			alard.Tracks++
		}
	}

	// Now we need to sort all the lists:

	// Artists
	sort.Sort(artistSorter{globalListCache.artistsByName, func(a, b *ArtistData) bool {
		return NaturalLess(strings.ToLower(a.Name), strings.ToLower(b.Name))
	}})
	sort.Sort(artistSorter{globalListCache.artistsByTracks, func(a, b *ArtistData) bool {
		return a.Tracks < b.Tracks
	}})
	sort.Sort(artistSorter{globalListCache.artistsByAlbums, func(a, b *ArtistData) bool {
		return a.Albums < b.Albums
	}})

	sort.Sort(sort.Reverse(artistSorter{globalListCache.artistsByNameRev, func(a, b *ArtistData) bool {
		return NaturalLess(strings.ToLower(a.Name), strings.ToLower(b.Name))
	}}))
	sort.Sort(sort.Reverse(artistSorter{globalListCache.artistsByTracksRev, func(a, b *ArtistData) bool {
		return a.Tracks < b.Tracks
	}}))
	sort.Sort(sort.Reverse(artistSorter{globalListCache.artistsByAlbumsRev, func(a, b *ArtistData) bool {
		return a.Albums < b.Albums
	}}))

	// Album artist list
	for _, ald := range globalListCache.albums {
		sort.Sort(artistSorter{ald.artistsByName, func(a, b *ArtistData) bool {
			return NaturalLess(strings.ToLower(a.Name), strings.ToLower(b.Name))
		}})
		sort.Sort(artistSorter{ald.artistsByTracks, func(a, b *ArtistData) bool {
			return a.Tracks < b.Tracks
		}})
		sort.Sort(artistSorter{ald.artistsByAlbums, func(a, b *ArtistData) bool {
			return a.Albums < b.Albums
		}})

		sort.Sort(sort.Reverse(artistSorter{ald.artistsByNameRev, func(a, b *ArtistData) bool {
			return NaturalLess(strings.ToLower(a.Name), strings.ToLower(b.Name))
		}}))
		sort.Sort(sort.Reverse(artistSorter{ald.artistsByTracksRev, func(a, b *ArtistData) bool {
			return a.Tracks < b.Tracks
		}}))
		sort.Sort(sort.Reverse(artistSorter{ald.artistsByAlbumsRev, func(a, b *ArtistData) bool {
			return a.Albums < b.Albums
		}}))
	}

	// Albums
	sort.Sort(albumSorter{globalListCache.albumsByName, func(a, b *AlbumData) bool {
		return NaturalLess(strings.ToLower(a.Name), strings.ToLower(b.Name))
	}})
	sort.Sort(albumSorter{globalListCache.albumsByTracks, func(a, b *AlbumData) bool {
		return a.Tracks < b.Tracks
	}})
	sort.Sort(albumSorter{globalListCache.albumsByYear, func(a, b *AlbumData) bool {
		return a.Year < b.Year
	}})

	sort.Sort(sort.Reverse(albumSorter{globalListCache.albumsByNameRev, func(a, b *AlbumData) bool {
		return NaturalLess(strings.ToLower(a.Name), strings.ToLower(b.Name))
	}}))
	sort.Sort(sort.Reverse(albumSorter{globalListCache.albumsByTracksRev, func(a, b *AlbumData) bool {
		return a.Tracks < b.Tracks
	}}))
	sort.Sort(sort.Reverse(albumSorter{globalListCache.albumsByYearRev, func(a, b *AlbumData) bool {
		return a.Year < b.Year
	}}))

	// Artist album list
	for _, ard := range globalListCache.artists {
		sort.Sort(albumSorter{ard.albumsByName, func(a, b *AlbumData) bool {
			return NaturalLess(strings.ToLower(a.Name), strings.ToLower(b.Name))
		}})
		sort.Sort(albumSorter{ard.albumsByTracks, func(a, b *AlbumData) bool {
			return a.Tracks < b.Tracks
		}})
		sort.Sort(albumSorter{ard.albumsByYear, func(a, b *AlbumData) bool {
			return a.Year < b.Year
		}})

		sort.Sort(sort.Reverse(albumSorter{ard.albumsByNameRev, func(a, b *AlbumData) bool {
			return NaturalLess(strings.ToLower(a.Name), strings.ToLower(b.Name))
		}}))
		sort.Sort(sort.Reverse(albumSorter{ard.albumsByTracksRev, func(a, b *AlbumData) bool {
			return a.Tracks < b.Tracks
		}}))
		sort.Sort(sort.Reverse(albumSorter{ard.albumsByYearRev, func(a, b *AlbumData) bool {
			return a.Year < b.Year
		}}))
	}

	// That was a lot of sorting.
	return nil
}

// Internal stuff below this point.

func copyArtistList(src []*ArtistData) []ArtistData {
	dest := make([]ArtistData, len(src))
	for i, v := range src {
		dest[i] = *v
	}
	return dest
}

func copyAlbumList(src []*AlbumData) []AlbumData {
	dest := make([]AlbumData, len(src))
	for i, v := range src {
		dest[i] = *v
	}
	return dest
}

type artistSorter struct {
	l   []*ArtistData
	cmp func(*ArtistData, *ArtistData) bool
}

func (s artistSorter) Len() int           { return len(s.l) }
func (s artistSorter) Swap(i, j int)      { s.l[i], s.l[j] = s.l[j], s.l[i] }
func (s artistSorter) Less(i, j int) bool { return s.cmp(s.l[i], s.l[j]) }

type albumSorter struct {
	l   []*AlbumData
	cmp func(*AlbumData, *AlbumData) bool
}

func (s albumSorter) Len() int           { return len(s.l) }
func (s albumSorter) Swap(i, j int)      { s.l[i], s.l[j] = s.l[j], s.l[i] }
func (s albumSorter) Less(i, j int) bool { return s.cmp(s.l[i], s.l[j]) }

var globalListCache = &listCache{}

type listCache struct {
	sync.RWMutex

	artists map[string]*listCacheArtist

	artistsByName   []*ArtistData
	artistsByTracks []*ArtistData
	artistsByAlbums []*ArtistData

	artistsByNameRev   []*ArtistData
	artistsByTracksRev []*ArtistData
	artistsByAlbumsRev []*ArtistData

	albums map[string]*listCacheAlbum

	albumsByName   []*AlbumData
	albumsByTracks []*AlbumData
	albumsByYear   []*AlbumData

	albumsByNameRev   []*AlbumData
	albumsByTracksRev []*AlbumData
	albumsByYearRev   []*AlbumData
}

type listCacheArtist struct {
	data *ArtistData

	albums map[string]*AlbumData

	albumsByName   []*AlbumData
	albumsByTracks []*AlbumData
	albumsByYear   []*AlbumData

	albumsByNameRev   []*AlbumData
	albumsByTracksRev []*AlbumData
	albumsByYearRev   []*AlbumData
}

type listCacheAlbum struct {
	data *AlbumData

	artists map[string]*ArtistData

	artistsByName   []*ArtistData
	artistsByTracks []*ArtistData
	artistsByAlbums []*ArtistData

	artistsByNameRev   []*ArtistData
	artistsByTracksRev []*ArtistData
	artistsByAlbumsRev []*ArtistData
}

type listCacheTrack struct {
	Artist string
	Album  string
	Year   int
}
