package edit

// Options is what the caller decides rather than the applier.
//
// One field, because the other candidate turned out not to be one. How many edits
// were asked for is known only to whoever asked, so it cannot come from the patch.
type Options struct {
	// MaxHunks is how many changes the caller asked for. A reply carrying more is
	// well formed and edits lines nobody mentioned; measured, asking two models for
	// two hunks produced replies with 27, 57, 59, 68 and 71.
	//
	// Zero means one, because one is what every measured probe asked for and a
	// zero-value Options should behave like the case the numbers describe.
	MaxHunks int
}

// hunks is MaxHunks with its zero value resolved.
func (o Options) hunks() int {
	if o.MaxHunks < 1 {
		return 1
	}
	return o.MaxHunks
}
