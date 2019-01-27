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

import "errors"

func AddTrack(t *Track, art *TrackArt) error {
	r, err := trackInsert.Exec(
		t.Name, t.Artist, t.AlbumArtist, t.Album, t.Composer, t.Publisher,
		t.Comments, t.Disk, t.Track, t.Length, t.Genre, t.Year, t.Path)
	if err != nil {
		return err
	}
	idx, err := r.LastInsertId()
	if err != nil {
		return err
	}
	t.ID = int(idx)

	if art != nil {
		_, err := trackArtUpdate.Exec(t.ID, art.Art, art.ArtMIME)
		if err != nil {
			return err
		}
	}
	return nil
}

func ReplaceTrack(t *Track, art *TrackArt) error {
	if t.ID <= 0 {
		return AddTrack(t, art)
	}

	_, err := trackBulkLoad.Exec(
		t.ID, t.Name, t.Artist, t.AlbumArtist, t.Album, t.Composer, t.Publisher,
		t.Comments, t.Disk, t.Track, t.Length, t.Genre, t.Year, t.Path)
	if err != nil {
		return err
	}

	if art != nil {
		_, err := trackArtUpdate.Exec(t.ID, art.Art, art.ArtMIME)
		if err != nil {
			return err
		}
	}
	return nil
}

func UpdateTrack(t *Track) error {
	_, err := trackUpdate.Exec(
		t.ID, t.Name, t.Artist, t.AlbumArtist, t.Album, t.Composer, t.Publisher,
		t.Comments, t.Disk, t.Track, t.Length, t.Genre, t.Year, t.Path)
	return err
}

func UpdateTrackArt(id int, art *TrackArt) error {
	_, err := trackArtUpdate.Exec(id, art.Art, art.ArtMIME)
	return err
}

func QueryTracks(artist, album string, sort sortOrder, asc bool) ([]int, error) {
	queryH, ok := queries[sort]
	if !ok {
		return nil, errors.New("Invalid track sort order.")
	}
	query := queryH.Dsc
	if asc {
		query = queryH.Asc
	}

	rows, err := query.Query(artist, album)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]int, 0)
	for rows.Next() {
		t := new(int)
		err := rows.Scan(t)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, nil
}

func QueryTrack(id int) (*Track, error) {
	row := trackQuery.QueryRow(id)
	t := &trackReader{&Track{}, 0}
	//              1  2  3  4  5  6  7  8  9  0  1  2  3  4
	err := row.Scan(t, t, t, t, t, t, t, t, t, t, t, t, t, t)
	if err != nil {
		return nil, err
	}
	return t.t, nil
}

func QueryTrackArt(id int) (*TrackArt, error) {
	row := trackArtQuery.QueryRow(id)
	art := &TrackArt{}
	err := row.Scan(&art.Art, &art.ArtMIME)
	if err != nil {
		return nil, err
	}
	return art, nil
}

func Dump() ([]struct {
	T *Track
	A *TrackArt
}, error) {
	ids, err := QueryTracks("", "", SortID, true)
	if err != nil {
		return nil, err
	}

	rtn := make([]struct {
		T *Track
		A *TrackArt
	}, len(ids))
	for i, id := range ids {
		rtn[i].T, err = QueryTrack(id)
		if err != nil {
			return nil, err
		}
		rtn[i].A, err = QueryTrackArt(id)
		if err != nil {
			return nil, err
		}
	}
	return rtn, nil
}

type Track struct {
	ID   int    // 1
	Name string // 2

	Artist      string // 3
	AlbumArtist string // 4
	Album       string // 5

	Composer  string // 6
	Publisher string // 7

	Comments string // 8

	Disk  int // 9
	Track int // 0

	Length int // 1

	Genre string // 2
	Year  int    // 3

	Path string // 4
}

type TrackArt struct {
	Art     []byte // 5
	ArtMIME string // 6
}

type trackReader struct {
	t *Track
	s int
}

// false means []byte or string, true integer.
var trackTypMap = []bool{
	true,  // 1
	false, // 2
	false, // 3
	false, // 4
	false, // 5
	false, // 6
	false, // 7
	false, // 8
	true,  // 9
	true,  // 0
	true,  // 1
	false, // 2
	true,  // 3
	false, // 4
}

func (t *trackReader) Scan(src interface{}) error {
	vs, vb, vi := "", []byte{}, 0
	if t.s >= len(trackTypMap) {
		return nil
	}
	if trackTypMap[t.s] {
		v, ok := src.(int64)
		if ok {
			vi = int(v)
		}
	} else {
		ok := false
		vb, ok = src.([]byte)
		if ok {
			vs = string(vb)
		}
	}

	switch t.s {
	case 0:
		t.t.ID = vi
	case 1:
		t.t.Name = vs
	case 2:
		t.t.Artist = vs
	case 3:
		t.t.AlbumArtist = vs
	case 4:
		t.t.Album = vs
	case 5:
		t.t.Composer = vs
	case 6:
		t.t.Publisher = vs
	case 7:
		t.t.Comments = vs
	case 8:
		t.t.Disk = vi
	case 9:
		t.t.Track = vi
	case 10:
		t.t.Length = vi
	case 11:
		t.t.Genre = vs
	case 12:
		t.t.Year = vi
	case 13:
		t.t.Path = vs
	}
	t.s++
	return nil
}
