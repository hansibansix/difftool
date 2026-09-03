package diff

import (
	"reflect"
	"testing"
)

func TestChangedSpans(t *testing.T) {
	sa, sb := ChangedSpans([]rune("abcdef"), []rune("abXdef"))
	if !reflect.DeepEqual(sa, []Span{{2, 3}}) || !reflect.DeepEqual(sb, []Span{{2, 3}}) {
		t.Fatalf("got %v %v", sa, sb)
	}
	// pure insertion on the right
	sa, sb = ChangedSpans([]rune("abc"), []rune("abXXc"))
	if len(sa) != 0 || !reflect.DeepEqual(sb, []Span{{2, 4}}) {
		t.Fatalf("got %v %v", sa, sb)
	}
	// identical
	sa, sb = ChangedSpans([]rune("same"), []rune("same"))
	if len(sa) != 0 || len(sb) != 0 {
		t.Fatalf("got %v %v", sa, sb)
	}
}
