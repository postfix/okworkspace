package agent

// go-udiff is pinned now (slice 1) but first USED in slice 5 (the D4 diff-
// locality / churn metric on propose→apply). Anchoring a blank import here keeps
// the module in go.mod across `go mod tidy` runs in intervening slices, so the
// pre-1.0 pin established in this slice does not silently drop before its first
// real consumer lands.
import _ "github.com/aymanbagabas/go-udiff"
