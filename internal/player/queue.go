package player

import "swipefi/internal/store"

// Queue manages an ordered list of tracks with a current position.
type Queue struct {
	tracks []store.Track
	pos    int
}

func NewQueue(tracks []store.Track) *Queue {
	return &Queue{tracks: tracks, pos: 0}
}

func (q *Queue) Current() *store.Track {
	if q.pos < 0 || q.pos >= len(q.tracks) {
		return nil
	}
	return &q.tracks[q.pos]
}

func (q *Queue) Next() *store.Track {
	if q.pos+1 >= len(q.tracks) {
		return nil
	}
	q.pos++
	return q.Current()
}

func (q *Queue) Prev() *store.Track {
	if q.pos-1 < 0 {
		return nil
	}
	q.pos--
	return q.Current()
}

func (q *Queue) RemoveCurrent() {
	if q.pos < 0 || q.pos >= len(q.tracks) {
		return
	}
	q.tracks = append(q.tracks[:q.pos], q.tracks[q.pos+1:]...)
	// Keep pos pointing at the next track (which slid into the current position)
	if q.pos >= len(q.tracks) && len(q.tracks) > 0 {
		q.pos = len(q.tracks) - 1
	}
}

func (q *Queue) Len() int {
	return len(q.tracks)
}

func (q *Queue) Position() int {
	return q.pos
}

func (q *Queue) Tracks() []store.Track {
	return q.tracks
}

// Reorder sets a new track order by IDs. The current track is preserved.
func (q *Queue) Reorder(ids []int64) {
	idxMap := make(map[int64]store.Track, len(q.tracks))
	for _, t := range q.tracks {
		idxMap[t.ID] = t
	}

	var currentID int64
	if cur := q.Current(); cur != nil {
		currentID = cur.ID
	}

	newTracks := make([]store.Track, 0, len(ids))
	for _, id := range ids {
		if t, ok := idxMap[id]; ok {
			newTracks = append(newTracks, t)
		}
	}

	q.tracks = newTracks

	// Restore position to current track
	q.pos = 0
	for i, t := range q.tracks {
		if t.ID == currentID {
			q.pos = i
			break
		}
	}
}

// UpdateCurrentPlayCount updates the play count of the current track in-memory.
func (q *Queue) UpdateCurrentPlayCount(count int) {
	if q.pos >= 0 && q.pos < len(q.tracks) {
		q.tracks[q.pos].PlayCount = count
	}
}

// SkipTo jumps to the track with the given ID, removing all tracks before it.
func (q *Queue) SkipTo(id int64) bool {
	for i, t := range q.tracks {
		if t.ID == id {
			q.tracks = q.tracks[i:]
			q.pos = 0
			return true
		}
	}
	return false
}
