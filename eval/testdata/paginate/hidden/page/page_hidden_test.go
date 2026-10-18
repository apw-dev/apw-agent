package page

import (
	"reflect"
	"testing"
)

func TestHiddenPage(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}
	cases := []struct {
		page, size int
		want       []string
	}{
		{1, 2, []string{"a", "b"}}, {2, 2, []string{"c", "d"}}, {3, 2, []string{"e"}}, {4, 2, nil},
		{0, 2, nil}, {-1, 2, nil}, {1, 0, nil}, {1, -3, nil}, {1, 5, items},
	}
	for _, c := range cases {
		if got := Page(items, c.page, c.size); !reflect.DeepEqual(got, c.want) {
			t.Fatalf("Page(page=%d, size=%d) = %v, want %v", c.page, c.size, got, c.want)
		}
	}
}
