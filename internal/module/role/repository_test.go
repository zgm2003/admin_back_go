package role

import (
	"reflect"
	"testing"
)

func TestDiffRolePermissionIDs(t *testing.T) {
	cases := []struct {
		name       string
		currentIDs []int64
		nextIDs    []int64
		wantAdd    []int64
		wantRemove []int64
	}{
		{
			name:       "grant page",
			currentIDs: []int64{},
			nextIDs:    []int64{2},
			wantAdd:    []int64{2},
			wantRemove: []int64{},
		},
		{
			name:       "grant button",
			currentIDs: []int64{2},
			nextIDs:    []int64{2, 3},
			wantAdd:    []int64{3},
			wantRemove: []int64{},
		},
		{
			name:       "remove button",
			currentIDs: []int64{2, 3},
			nextIDs:    []int64{2},
			wantAdd:    []int64{},
			wantRemove: []int64{3},
		},
		{
			name:       "remove page",
			currentIDs: []int64{2, 3},
			nextIDs:    []int64{},
			wantAdd:    []int64{},
			wantRemove: []int64{2, 3},
		},
		{
			name:       "dedupe and sort before diff",
			currentIDs: []int64{3, 2, 2, 0, -1},
			nextIDs:    []int64{4, 2, 4},
			wantAdd:    []int64{4},
			wantRemove: []int64{3},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotAdd, gotRemove := diffRolePermissionIDs(tc.currentIDs, tc.nextIDs)
			if !reflect.DeepEqual(gotAdd, tc.wantAdd) || !reflect.DeepEqual(gotRemove, tc.wantRemove) {
				t.Fatalf("diff mismatch\nwant add=%#v remove=%#v\n got add=%#v remove=%#v", tc.wantAdd, tc.wantRemove, gotAdd, gotRemove)
			}
		})
	}
}
