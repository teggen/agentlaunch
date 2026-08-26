package catalog

import (
	"reflect"
	"testing"
	"time"
)

// timeAt returns a fixed instant hours into a fixed day, so age arithmetic
// never depends on when the suite runs.
func timeAt(t *testing.T, hours int) time.Time {
	t.Helper()
	return time.Date(2026, 8, 26, hours, 0, 0, 0, time.UTC)
}

func TestIsFree(t *testing.T) {
	paid := Model{ID: "a/b", PromptPricePerM: 15, CompletionPricePerM: 75}
	free := Model{ID: "c/d"}
	if paid.IsFree() {
		t.Error("paid model reported as free")
	}
	if !free.IsFree() {
		t.Error("free model not reported as free")
	}
}

// TestUnknownPricingIsNeverFree is Landmine 4 at the type level: a model
// whose price could not be parsed has BOTH price fields at zero, which is
// indistinguishable from genuinely free without the flag. Reporting it free
// is the tool stating a price it does not know.
func TestUnknownPricingIsNeverFree(t *testing.T) {
	unknown := Model{ID: "a/b", PricingUnknown: true}
	if unknown.IsFree() {
		t.Error("a model with unparseable pricing was reported as free")
	}
}

func TestMatchesIDAndNameCaseInsensitively(t *testing.T) {
	m := Model{ID: "vendor/uniqueslug-model", Name: "Distinctive Title"}
	for _, term := range []string{"uniqueslug", "UNIQUESLUG", "distinctive", "Distinctive"} {
		if !m.Matches(term) {
			t.Errorf("Matches(%q) = false", term)
		}
	}
	if m.Matches("nothing-like-this") {
		t.Error("Matches matched an unrelated term")
	}
}

// TestModelHasNoJSONTags is the machine-checked half of the comment on Model.
// The on-disk cache marshals this type directly and existing files store Go
// field names, so adding snake_case tags would keep decoding (encoding/json
// matches case-insensitively) while zeroing every price — a $75/M model
// rendering free, which is Landmine 4's false claim by a new route.
func TestModelHasNoJSONTags(t *testing.T) {
	typ := reflect.TypeOf(Model{})
	for i := range typ.NumField() {
		f := typ.Field(i)
		if tag, ok := f.Tag.Lookup("json"); ok {
			t.Errorf("Model.%s carries a json tag %q; adding tags silently zeroes "+
				"every field in existing cache files", f.Name, tag)
		}
	}
}
