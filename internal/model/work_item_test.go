package model

import "testing"

func TestWorkItemValidate(t *testing.T) {
	tests := []struct {
		name    string
		item    WorkItem
		wantErr bool
	}{
		{
			name: "valid item",
			item: WorkItem{
				ID:             "local-demo-001",
				Type:           WorkItemTypeWriteDemoOutput,
				OutputFilename: "output.txt",
			},
		},
		{
			name: "missing id",
			item: WorkItem{
				Type:           WorkItemTypeWriteDemoOutput,
				OutputFilename: "output.txt",
			},
			wantErr: true,
		},
		{
			name: "missing type",
			item: WorkItem{
				ID:             "local-demo-001",
				OutputFilename: "output.txt",
			},
			wantErr: true,
		},
		{
			name: "unknown type is structurally valid",
			item: WorkItem{
				ID:             "local-demo-001",
				Type:           "unknown",
				OutputFilename: "output.txt",
			},
		},
		{
			name: "missing output filename",
			item: WorkItem{
				ID:   "local-demo-001",
				Type: WorkItemTypeWriteDemoOutput,
			},
			wantErr: true,
		},
		{
			name: "output filename contains directory",
			item: WorkItem{
				ID:             "local-demo-001",
				Type:           WorkItemTypeWriteDemoOutput,
				OutputFilename: "../outside.txt",
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.item.Validate()

			if test.wantErr && err == nil {
				t.Fatal("expected an error")
			}

			if !test.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
