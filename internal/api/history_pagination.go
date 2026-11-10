package api

import (
	"reflect"

	"github.com/danielgtaylor/huma/v2"
)

// navigation-and-lists R4-R6, context ADR 0009: both histories share this rollout contract.
const historyPaginationDescription = "Omit offset to preserve legacy cursor paging, including before/nextBefore and the default limit of 50. " +
	"An explicitly supplied offset, including offset=0, selects numbered mode; offset and before together return 422. " +
	"Numbered success includes a bounded page and total, always present and exact for all current filters before offset/limit, " +
	"including total: 0 for no matches. An out-of-range page is empty with the actual matching total. " +
	"total is omitted from legacy responses. During this contract-first stage, numbered mode temporarily returns 501; " +
	"no numbered success or count is implemented yet. Authorization and validation run before this temporary response."

// historyOffset preserves explicit zero: Huma does not support pointer query parameters.
// context ADR 0009: omitting offset keeps the legacy cursor mode.
type historyOffset struct {
	Value int64
	Set   bool
}

func (historyOffset) Schema(r huma.Registry) *huma.Schema {
	return r.Schema(reflect.TypeFor[int64](), false, "")
}

func (o *historyOffset) Receiver() reflect.Value {
	return reflect.ValueOf(&o.Value).Elem()
}

func (o *historyOffset) OnParamSet(set bool, _ any) {
	o.Set = set
}

// context ADR 0009: cursor and numbered page selections cannot be combined.
func validateHistoryPagination(ctx huma.Context) []error {
	u := ctx.URL()
	query := u.Query()
	if !query.Has("offset") {
		return nil
	}
	if query.Has("before") {
		return []error{&huma.ErrorDetail{Location: "query.before", Message: "must not be supplied with offset", Value: query.Get("before")}}
	}
	// Huma treats an empty query parameter as absent; explicit offset must be numeric.
	if query.Get("offset") == "" {
		return []error{&huma.ErrorDetail{Location: "query.offset", Message: "must be an integer", Value: ""}}
	}
	return nil
}
