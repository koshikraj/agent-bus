package bus

import (
	"fmt"
	"regexp"
	"strings"
)

var sessionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

// Address is where a message goes: agent, agent/session, or agent@node.
//
// Node is reserved for other machines. Agent names can't contain / or @, so
// the plain names in use today stay valid when that arrives.
type Address struct {
	Agent, Session, Node string
}

// ParseAddress splits a recipient such as "claude", "claude/api" or
// "claude/api@laptop".
func ParseAddress(s string) (Address, error) {
	var a Address
	rest, node, hasNode := strings.Cut(s, "@")
	agent, session, hasSession := strings.Cut(rest, "/")
	a.Agent, a.Session, a.Node = agent, session, node
	if a.Agent == "" || (hasNode && node == "") || (hasSession && !sessionPattern.MatchString(session)) {
		return Address{}, fmt.Errorf("%w: bad address %q (want agent or agent/session)", ErrInvalid, s)
	}
	return a, nil
}

func (a Address) String() string {
	s := a.Agent
	if a.Session != "" {
		s += "/" + a.Session
	}
	if a.Node != "" {
		s += "@" + a.Node
	}
	return s
}
