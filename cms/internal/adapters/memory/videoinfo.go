package memory

import (
	"context"
	"sync"

	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// VideoInfo is a fake ports.VideoInfo: every Vimeo video exists with a
// placeholder thumbnail, except the ids in Unavailable; Err, when set, is
// returned for every call (Vimeo unreachable).
type VideoInfo struct {
	mu          sync.Mutex
	Unavailable map[string]bool
	Err         error
	Calls       int
}

// Vimeo implements ports.VideoInfo.
func (v *VideoInfo) Vimeo(_ context.Context, id, hash string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.Calls++
	if v.Err != nil {
		return "", v.Err
	}
	if v.Unavailable[id] {
		return "", ports.ErrVideoUnavailable
	}
	return "https://i.vimeocdn.com/video/" + id + "-d_640", nil
}
