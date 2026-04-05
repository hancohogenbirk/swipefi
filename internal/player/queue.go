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
