package usecase

import (
	"context"
	"testing"

	"github.com/cryptox/notification-service/internal/domain"
)

type fakeRepo struct {
	created   []*domain.Notification
	listResp  []domain.Notification
	unread    int64
	markedOne [2]uint
	markedAll uint
}

func (r *fakeRepo) Create(_ context.Context, n *domain.Notification) error {
	n.ID = uint(len(r.created) + 1)
	r.created = append(r.created, n)
	return nil
}
func (r *fakeRepo) FindByUser(_ context.Context, _ uint, _ bool, _, _ int) ([]domain.Notification, int64, error) {
	return r.listResp, int64(len(r.listResp)), nil
}
func (r *fakeRepo) UnreadCount(context.Context, uint) (int64, error) { return r.unread, nil }
func (r *fakeRepo) MarkAsRead(_ context.Context, userID, id uint) error {
	r.markedOne = [2]uint{userID, id}
	return nil
}
func (r *fakeRepo) MarkAllRead(_ context.Context, userID uint) error { r.markedAll = userID; return nil }

type fakeBroadcaster struct {
	channel string
	calls   int
}

func (b *fakeBroadcaster) Broadcast(channel string, _ interface{}) { b.channel = channel; b.calls++ }

func TestNotify_PersistsAndBroadcasts(t *testing.T) {
	repo := &fakeRepo{}
	bc := &fakeBroadcaster{}
	uc := NewNotificationUseCase(repo, bc)

	if err := uc.Notify(context.Background(), 42, "ORDER_FILLED", "Filled", "your order filled", "BTC_USDT"); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("expected 1 persisted notification, got %d", len(repo.created))
	}
	if bc.calls != 1 || bc.channel != "notification@42" {
		t.Fatalf("expected broadcast to notification@42, got %q (%d calls)", bc.channel, bc.calls)
	}
}

func TestUseCase_Delegations(t *testing.T) {
	repo := &fakeRepo{unread: 3, listResp: []domain.Notification{{ID: 1}, {ID: 2}}}
	uc := NewNotificationUseCase(repo, nil)

	if n, total, _ := uc.List(context.Background(), 1, 1, 20, true); total != 2 || len(n) != 2 {
		t.Fatalf("List delegation wrong: total=%d len=%d", total, len(n))
	}
	if c, _ := uc.UnreadCount(context.Background(), 1); c != 3 {
		t.Fatalf("UnreadCount = %d, want 3", c)
	}
	_ = uc.MarkRead(context.Background(), 9, 5)
	if repo.markedOne != [2]uint{9, 5} {
		t.Fatalf("MarkRead delegation wrong: %v", repo.markedOne)
	}
	_ = uc.MarkAllRead(context.Background(), 9)
	if repo.markedAll != 9 {
		t.Fatalf("MarkAllRead delegation wrong: %v", repo.markedAll)
	}
}
