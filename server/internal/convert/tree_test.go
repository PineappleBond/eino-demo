package convert

import (
	"testing"

	"github.com/google/uuid"

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
