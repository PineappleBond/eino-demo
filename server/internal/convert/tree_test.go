package convert

import (
	"testing"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
	"github.com/PineappleBond/eino-demo-dev/server/internal/types"
)

func makeConv(id string, parentID *string) model.Conversation {
	c := model.Conversation{
		Title:  id,
		Status: "active",
	}
	c.ID = uuid.MustParse(id)
	if parentID != nil {
		pid := uuid.MustParse(*parentID)
		c.ParentConversationID = &pid
	}
	return c
}

func ptr(s string) *string { return &s }

func TestBuildConversationTree(t *testing.T) {
	tests := []struct {
		name      string
		convs     []model.Conversation
		wantRoots int
		wantTotal int
	}{
		{
			name: "flat list no parents",
			convs: []model.Conversation{
				makeConv("00000000-0000-0000-0000-000000000001", nil),
				makeConv("00000000-0000-0000-0000-000000000002", nil),
			},
			wantRoots: 2,
			wantTotal: 2,
		},
		{
			name: "one level of children",
			convs: []model.Conversation{
				makeConv("00000000-0000-0000-0000-000000000001", nil),
				makeConv("00000000-0000-0000-0000-000000000002", ptr("00000000-0000-0000-0000-000000000001")),
				makeConv("00000000-0000-0000-0000-000000000003", ptr("00000000-0000-0000-0000-000000000001")),
			},
			wantRoots: 1,
			wantTotal: 3,
		},
		{
			name: "nested three levels",
			convs: []model.Conversation{
				makeConv("00000000-0000-0000-0000-000000000001", nil),
				makeConv("00000000-0000-0000-0000-000000000002", ptr("00000000-0000-0000-0000-000000000001")),
				makeConv("00000000-0000-0000-0000-000000000003", ptr("00000000-0000-0000-0000-000000000002")),
			},
			wantRoots: 1,
			wantTotal: 3,
		},
		{
			name: "orphan node becomes root",
			convs: []model.Conversation{
				makeConv("00000000-0000-0000-0000-000000000002", ptr("00000000-0000-0000-0000-000000000099")),
			},
			wantRoots: 1,
			wantTotal: 1,
		},
		{
			name: "circular reference broken",
			convs: []model.Conversation{
				makeConv("00000000-0000-0000-0000-000000000001", ptr("00000000-0000-0000-0000-000000000002")),
				makeConv("00000000-0000-0000-0000-000000000002", ptr("00000000-0000-0000-0000-000000000001")),
			},
			wantRoots: 2,
			wantTotal: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roots := BuildConversationTree(tt.convs)
			if len(roots) != tt.wantRoots {
				t.Errorf("roots count = %d, want %d", len(roots), tt.wantRoots)
			}
			total := countNodesTypes(roots)
			if total != tt.wantTotal {
				t.Errorf("total nodes = %d, want %d", total, tt.wantTotal)
			}
		})
	}
}

func countNodesTypes(nodes []types.Conversation) int {
	n := len(nodes)
	for _, node := range nodes {
		if node.Children != nil {
			n += countNodesTypes(*node.Children)
		}
	}
	return n
}

func makeConvWithTime(id string, parentID *string, updatedAt time.Time) model.Conversation {
	c := model.Conversation{
		Title:     id,
		Status:    "active",
		UpdatedAt: updatedAt,
	}
	c.ID = uuid.MustParse(id)
	if parentID != nil {
		pid := uuid.MustParse(*parentID)
		c.ParentConversationID = &pid
	}
	return c
}

func TestBuildConversationTree_OrderByUpdatedAt(t *testing.T) {
	now := time.Now()
	parentID := "00000000-0000-0000-0000-000000000001"
	// Input simulates DB result (updated_at DESC): newest first
	convs := []model.Conversation{
		makeConvWithTime("00000000-0000-0000-0000-000000000002", &parentID, now),                // child of id1, newest
		makeConvWithTime("00000000-0000-0000-0000-000000000004", nil, now),                       // root, newest
		makeConvWithTime("00000000-0000-0000-0000-000000000003", nil, now.Add(-1*time.Hour)),     // root, 1h ago
		makeConvWithTime(parentID, nil, now.Add(-2*time.Hour)),                                    // root, oldest
	}

	roots := BuildConversationTree(convs)

	if len(roots) != 3 {
		t.Fatalf("roots count = %d, want 3", len(roots))
	}

	// Roots should be ordered by UpdatedAt DESC: id4 (now), id3 (-1h), id1 (-2h)
	lastChar := func(id openapi_types.UUID) string { s := id.String(); return s[len(s)-1:] }

	if lastChar(roots[0].Id) != "4" {
		t.Errorf("first root should be id=...4 (newest), got id=...%s", lastChar(roots[0].Id))
	}
	if lastChar(roots[1].Id) != "3" {
		t.Errorf("second root should be id=...3, got id=...%s", lastChar(roots[1].Id))
	}
	if lastChar(roots[2].Id) != "1" {
		t.Errorf("third root should be id=...1 (oldest), got id=...%s", lastChar(roots[2].Id))
	}

	// Children of id1 should also be ordered by UpdatedAt DESC
	parent := roots[2]
	if parent.Children == nil || len(*parent.Children) != 1 {
		t.Fatalf("expected 1 child for id1")
	}
}
