package ipcs

import (
	"context"

	"github.com/containerd/containerd/content"
)

// Status returns the status of the provided ref.
func (s *store) Status(ctx context.Context, ref string) (content.Status, error) {
	panic("unimplemented")
	return content.Status{}, nil
}

// ListStatuses returns the status of any active ingestions whose ref match the
// provided regular expression. If empty, all active ingestions will be
// returned.
func (s *store) ListStatuses(ctx context.Context, filters ...string) ([]content.Status, error) {
	panic("unimplemented")
	return nil, nil
}

// Abort completely cancels the ingest operation targeted by ref.
func (s *store) Abort(ctx context.Context, ref string) error {
	panic("unimplemented")
	return nil
}
