package player

import (
	"testing"

	"swipefi/internal/store"
)

// makeTracks creates a slice of n tracks with sequential IDs and titles.
func makeTracks(n int) []store.Track {
	tracks := make([]store.Track, n)
	for i := range tracks {
		tracks[i] = store.Track{
			ID:    int64(i + 1),
			Title: "Track " + string(rune('A'+i)),
		}
	}
	return tracks
}

func TestNewQueue(t *testing.T) {
	tracks := makeTracks(3)
	q := NewQueue(tracks)

	if q.Position() != 0 {
		t.Errorf("expected position 0, got %d", q.Position())
	}
	if q.Len() != 3 {
		t.Errorf("expected len 3, got %d", q.Len())
	}
}

func TestCurrent(t *testing.T) {
	tracks := makeTracks(3)
	q := NewQueue(tracks)

	cur := q.Current()
	if cur == nil {
		t.Fatal("expected non-nil current track")
	}
	if cur.ID != 1 {
		t.Errorf("expected track ID 1, got %d", cur.ID)
	}
}

func TestCurrentEmptyQueue(t *testing.T) {
	q := NewQueue(nil)
	if q.Current() != nil {
		t.Error("expected nil from Current on empty queue")
	}
}

func TestNext(t *testing.T) {
	tracks := makeTracks(3)
	q := NewQueue(tracks)

	next := q.Next()
	if next == nil {
		t.Fatal("expected non-nil next track")
	}
	if next.ID != 2 {
		t.Errorf("expected track ID 2, got %d", next.ID)
	}
	if q.Position() != 1 {
		t.Errorf("expected position 1, got %d", q.Position())
	}
}

func TestNextAtEnd(t *testing.T) {
	tracks := makeTracks(2)
	q := NewQueue(tracks)

	q.Next() // advance to last
	result := q.Next()
	if result != nil {
		t.Errorf("expected nil at end of queue, got track ID %d", result.ID)
	}
	// Position should remain at last element
	if q.Position() != 1 {
		t.Errorf("expected position to stay at 1, got %d", q.Position())
	}
}

func TestPrev(t *testing.T) {
	tracks := makeTracks(3)
	q := NewQueue(tracks)

	q.Next() // move to pos 1
	prev := q.Prev()
	if prev == nil {
		t.Fatal("expected non-nil prev track")
	}
	if prev.ID != 1 {
		t.Errorf("expected track ID 1, got %d", prev.ID)
	}
	if q.Position() != 0 {
		t.Errorf("expected position 0, got %d", q.Position())
	}
}

func TestPrevAtStart(t *testing.T) {
	tracks := makeTracks(3)
	q := NewQueue(tracks)

	result := q.Prev()
	if result != nil {
		t.Errorf("expected nil at start of queue, got track ID %d", result.ID)
	}
	if q.Position() != 0 {
		t.Errorf("expected position to stay at 0, got %d", q.Position())
	}
}

func TestRemoveCurrentMiddle(t *testing.T) {
	// Remove a middle track: position should point to the track that slid in.
	tracks := makeTracks(3) // IDs: 1, 2, 3
	q := NewQueue(tracks)
	q.Next() // pos = 1, current = track 2

	q.RemoveCurrent()

	if q.Len() != 2 {
		t.Errorf("expected len 2 after remove, got %d", q.Len())
	}
	if q.Position() != 1 {
		t.Errorf("expected position 1 after remove, got %d", q.Position())
	}
	cur := q.Current()
	if cur == nil {
		t.Fatal("expected non-nil current after remove")
	}
	if cur.ID != 3 {
		t.Errorf("expected current track ID 3 (next slid in), got %d", cur.ID)
	}
}

func TestRemoveCurrentLast(t *testing.T) {
	// Remove the last track: position should adjust to the new last element.
	tracks := makeTracks(3) // IDs: 1, 2, 3
	q := NewQueue(tracks)
	q.Next()
	q.Next() // pos = 2, current = track 3

	q.RemoveCurrent()

	if q.Len() != 2 {
		t.Errorf("expected len 2 after remove, got %d", q.Len())
	}
	if q.Position() != 1 {
		t.Errorf("expected position adjusted to 1 after removing last, got %d", q.Position())
	}
	cur := q.Current()
	if cur == nil {
		t.Fatal("expected non-nil current after remove")
	}
	if cur.ID != 2 {
		t.Errorf("expected current track ID 2, got %d", cur.ID)
	}
}

func TestRemoveCurrentOnlyItem(t *testing.T) {
	tracks := makeTracks(1)
	q := NewQueue(tracks)

	q.RemoveCurrent()

	if q.Len() != 0 {
		t.Errorf("expected empty queue after removing only item, got len %d", q.Len())
	}
	if q.Current() != nil {
		t.Error("expected nil current on empty queue after remove")
	}
}

func TestReorder(t *testing.T) {
	tracks := makeTracks(4) // IDs: 1, 2, 3, 4
	q := NewQueue(tracks)
	q.Next() // pos = 1, current = track 2

	// Reverse order
	q.Reorder([]int64{4, 3, 2, 1})

	if q.Len() != 4 {
		t.Errorf("expected len 4 after reorder, got %d", q.Len())
	}
	// Current track (ID=2) should now be at index 2
	if q.Position() != 2 {
		t.Errorf("expected position 2 after reorder, got %d", q.Position())
	}
	cur := q.Current()
	if cur == nil {
		t.Fatal("expected non-nil current after reorder")
	}
	if cur.ID != 2 {
		t.Errorf("expected current track to still be ID 2 after reorder, got %d", cur.ID)
	}
}

func TestReorderPreservesCurrentTrack(t *testing.T) {
	tests := []struct {
		name        string
		startPos    int // number of Next() calls from start
		newOrder    []int64
		wantTrackID int64
		wantPos     int
	}{
		{
			name:        "current at start moves to end",
			startPos:    0, // current = track 1
			newOrder:    []int64{2, 3, 1},
			wantTrackID: 1,
			wantPos:     2,
		},
		{
			name:        "current at end moves to start",
			startPos:    2, // current = track 3
			newOrder:    []int64{3, 1, 2},
			wantTrackID: 3,
			wantPos:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := NewQueue(makeTracks(3))
			for i := 0; i < tt.startPos; i++ {
				q.Next()
			}

			q.Reorder(tt.newOrder)

			if q.Position() != tt.wantPos {
				t.Errorf("position: want %d, got %d", tt.wantPos, q.Position())
			}
			cur := q.Current()
			if cur == nil {
				t.Fatal("expected non-nil current after reorder")
			}
			if cur.ID != tt.wantTrackID {
				t.Errorf("current track ID: want %d, got %d", tt.wantTrackID, cur.ID)
			}
		})
	}
}

func TestSkipTo(t *testing.T) {
	tests := []struct {
		name     string
		skipToID int64
		wantOk   bool
		wantPos  int
	}{
		{"skip to first", 1, true, 0},
		{"skip to middle", 2, true, 1},
		{"skip to last", 3, true, 2},
		{"invalid ID returns false", 99, false, 0}, // pos unchanged
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := NewQueue(makeTracks(3))

			ok := q.SkipTo(tt.skipToID)

			if ok != tt.wantOk {
				t.Errorf("SkipTo(%d): want ok=%v, got ok=%v", tt.skipToID, tt.wantOk, ok)
			}
			if ok && q.Position() != tt.wantPos {
				t.Errorf("position after SkipTo(%d): want %d, got %d", tt.skipToID, tt.wantPos, q.Position())
			}
			if ok {
				cur := q.Current()
				if cur == nil || cur.ID != tt.skipToID {
					t.Errorf("current after SkipTo(%d): want ID %d", tt.skipToID, tt.skipToID)
				}
			}
		})
	}
}

func TestSkipToInvalidIDDoesNotMovePosition(t *testing.T) {
	q := NewQueue(makeTracks(3))
	q.Next() // pos = 1

	ok := q.SkipTo(99)

	if ok {
		t.Error("expected SkipTo with invalid ID to return false")
	}
	if q.Position() != 1 {
		t.Errorf("position should be unchanged at 1, got %d", q.Position())
	}
}

func TestUpdateCurrentPlayCount(t *testing.T) {
	tracks := makeTracks(3)
	q := NewQueue(tracks)
	q.Next() // pos = 1

	q.UpdateCurrentPlayCount(5)

	cur := q.Current()
	if cur == nil {
		t.Fatal("expected non-nil current")
	}
	if cur.PlayCount != 5 {
		t.Errorf("expected PlayCount 5, got %d", cur.PlayCount)
	}
	// Other tracks should be unaffected
	if q.tracks[0].PlayCount != 0 {
		t.Errorf("track[0] PlayCount should be 0, got %d", q.tracks[0].PlayCount)
	}
	if q.tracks[2].PlayCount != 0 {
		t.Errorf("track[2] PlayCount should be 0, got %d", q.tracks[2].PlayCount)
	}
}

func TestRemoveByID_AfterCurrent(t *testing.T) {
	q := NewQueue(makeTracks(4))
	q.Next() // pos=1, current=2

	if !q.RemoveByID(3) {
		t.Fatal("expected true")
	}
	if q.Len() != 3 {
		t.Errorf("want len 3, got %d", q.Len())
	}
	if q.Position() != 1 {
		t.Errorf("want pos 1 (unchanged), got %d", q.Position())
	}
	if q.Current().ID != 2 {
		t.Errorf("want current 2, got %d", q.Current().ID)
	}
}

func TestRemoveByID_BeforeCurrent(t *testing.T) {
	q := NewQueue(makeTracks(4))
	q.Next()
	q.Next() // pos=2, current=3

	if !q.RemoveByID(1) {
		t.Fatal("expected true")
	}
	if q.Len() != 3 {
		t.Errorf("want len 3, got %d", q.Len())
	}
	if q.Position() != 1 {
		t.Errorf("want pos 1 (decremented), got %d", q.Position())
	}
	if q.Current().ID != 3 {
		t.Errorf("want current still 3, got %d", q.Current().ID)
	}
}

func TestRemoveByID_CurrentTrack(t *testing.T) {
	q := NewQueue(makeTracks(3))
	q.Next() // pos=1, current=2

	if !q.RemoveByID(2) {
		t.Fatal("expected true")
	}
	if q.Len() != 2 {
		t.Errorf("want len 2, got %d", q.Len())
	}
	if q.Current().ID != 3 {
		t.Errorf("want current 3 (slid in), got %d", q.Current().ID)
	}
}

func TestRemoveByID_CurrentAtEnd(t *testing.T) {
	q := NewQueue(makeTracks(3))
	q.Next()
	q.Next() // pos=2, current=3

	if !q.RemoveByID(3) {
		t.Fatal("expected true")
	}
	if q.Len() != 2 {
		t.Errorf("want len 2, got %d", q.Len())
	}
	if q.Position() != 1 {
		t.Errorf("want pos 1 (clamped), got %d", q.Position())
	}
	if q.Current().ID != 2 {
		t.Errorf("want current 2, got %d", q.Current().ID)
	}
}

func TestRemoveByID_NonExistent(t *testing.T) {
	q := NewQueue(makeTracks(3))
	q.Next()

	if q.RemoveByID(99) {
		t.Error("expected false for non-existent ID")
	}
	if q.Len() != 3 {
		t.Errorf("want len unchanged, got %d", q.Len())
	}
}

func TestRemoveByID_LastRemaining(t *testing.T) {
	q := NewQueue(makeTracks(1))

	if !q.RemoveByID(1) {
		t.Fatal("expected true")
	}
	if q.Len() != 0 {
		t.Errorf("want empty, got %d", q.Len())
	}
	if q.Current() != nil {
		t.Error("want nil current")
	}
}

func TestLenAndPosition(t *testing.T) {
	tracks := makeTracks(4)
	q := NewQueue(tracks)

	if q.Len() != 4 {
		t.Errorf("Len: want 4, got %d", q.Len())
	}
	if q.Position() != 0 {
		t.Errorf("Position: want 0, got %d", q.Position())
	}

	q.Next()
	q.Next()

	if q.Len() != 4 {
		t.Errorf("Len after advancing: want 4, got %d", q.Len())
	}
	if q.Position() != 2 {
		t.Errorf("Position after two Next calls: want 2, got %d", q.Position())
	}
}
