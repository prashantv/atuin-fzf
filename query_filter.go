package main

// queryFilterMode controls how filtering is implemented.
type queryFilterMode int

const (
	queryFilterFzf   queryFilterMode = iota // fzf filters locally
	queryFilterAtuin                        // atuin re-searches on each keystroke
)

func (q queryFilterMode) toggle() queryFilterMode {
	if q == queryFilterAtuin {
		return queryFilterFzf
	}
	return queryFilterAtuin
}

func (q queryFilterMode) promptChar() string {
	if q == queryFilterAtuin {
		return "|"
	}
	return ">"
}

// changeBind returns the fzf bind/unbind action for the change event.
func (q queryFilterMode) changeBind() string {
	if q == queryFilterAtuin {
		return "rebind"
	}
	return "unbind"
}

// atuinFilter returns the query to pass to atuin.
func (q queryFilterMode) atuinFilter() string {
	if q == queryFilterAtuin {
		return " {q}"
	}
	return ""
}

func (q queryFilterMode) promptLabel() string {
	// The prompt label indicates what it will toggle to.
	if q.toggle() == queryFilterAtuin {
		return "atuin search"
	}
	return "fzf filter"
}
