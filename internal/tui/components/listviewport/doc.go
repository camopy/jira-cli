// Package listviewport is a scrollable, single-selection list of pre-rendered
// rows. Its defining property is that the selected item (cursor) is tracked
// independently of the scroll offset: moving the cursor adjusts the offset only
// enough to keep the selection visible, and a resize never loses the selection.
// Rows are opaque strings, so the component is decoupled from how a caller
// styles them — it owns only the selection/scroll math and an optional
// scrollbar.
package listviewport
