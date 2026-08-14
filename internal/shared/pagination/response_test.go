package pagination

import (
	"encoding/json"
	"testing"
)

func TestResultJSONKeepsEmptyListAndCompletePage(t *testing.T) {
	payload, err := json.Marshal(Result[int]{
		List: []int{},
		Page: Page{
			PageSize:    20,
			CurrentPage: 1,
			TotalPage:   0,
			Total:       0,
		},
	})
	if err != nil {
		t.Fatalf("marshal pagination result: %v", err)
	}

	const want = `{"list":[],"page":{"page_size":20,"current_page":1,"total_page":0,"total":0}}`
	if string(payload) != want {
		t.Fatalf("pagination JSON = %s, want %s", payload, want)
	}
}
