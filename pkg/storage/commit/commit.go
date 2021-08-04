package commit

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
)

/*type Resolver interface {
	ResolveHash(Hash) (Commit, error)
}

type Commit interface {
	Hash() Hash
	Author() Signature
	Message() Message
	Parents() []Hash
}*/

type Request interface {
	Author() Signature
	Message() Message
	Validate() error
}

type Signature interface {
	// Name describes the author's name (e.g. as per git config)
	// +required
	Name() string
	// Email describes the author's email (e.g. as per git config).
	// It is optional generally, but might be required by some specific
	// implementations.
	// +optional
	Email() string
	// When is the timestamp of the signature.
	// +optional
	When() *time.Time
	// The String() method must return a (ideally both human- and machine-
	// readable) concatenated string including the name and email (if
	// applicable) of the author.
	fmt.Stringer
}

type Message interface {
	// Title describes the change concisely, so it can be used e.g. as
	// a commit message or PR title. Certain implementations might enforce
	// character limits on this string.
	// +required
	Title() string
	// Description contains optional extra, more detailed information
	// about the change.
	// +optional
	Description() string
	// The String() method must return a (ideally both human- and machine-
	// readable) concatenated string including the title and description
	// (if applicable) of the author.
	fmt.Stringer
}

// Hash represents an immutable commit hash, represented as a set of "raw" bytes,
// probably from some hash function (e.g. SHA-1 or SHA-2-256), along with a well-defined
// string representation, e.g. Hexadecimal encoding.
type Hash interface {
	Hash() []byte
	// TODO: Rename to encoded and keep fmt.Stringer a debug print?
	String() string

	// RefSource returns the source of this computed Hash lock. Can be nil,
	// in case this doesn't have a symbolic source. This can be used for consumers
	// to understand how this immutable revision was computed.
	// TODO: Do we need this?
	// RefSource() Ref
}

func WithHash(ctx context.Context, h Hash) context.Context {
	if h == nil {
		return ctx
	}
	return context.WithValue(ctx, hashCtxKey, h)
}

func GetHash(ctx context.Context) (Hash, bool) {
	h, ok := ctx.Value(hashCtxKey).(Hash)
	return h, ok
}

type hashCtxKeyStruct struct{}

var hashCtxKey = hashCtxKeyStruct{}

type RefResolver interface {
	ResolveRef(Ref) (Hash, error)
	// GetRef extracts the Ref from the context, and if empty,
	// defaults it to the default Ref.
	GetRef(ctx context.Context) Ref
}

func SHA1(h [20]byte, src Ref) Hash {
	b := make([]byte, 20)
	copy(b, h[:])
	return &hash{hash: b, encoded: hex.EncodeToString(b), src: src}
}

func SHA1String(h string, src Ref) (Hash, bool) {
	b, err := hex.DecodeString(h)
	if err != nil {
		return nil, false
	}
	return &hash{hash: b, encoded: h, src: src}, true
}

func At(symbolic string) Ref {
	return &symbolicRef{RefTypeUnknown, symbolic, 0}
}

func Default() Ref {
	return AtBranch("") // Signifies the default branch
}

func AtBranch(b string) Ref {
	return Before(b, 0)
}

func Before(b string, n uint8) Ref {
	return &symbolicRef{RefTypeBranch, b, n}
}

func AtTag(t string) Ref {
	return &symbolicRef{RefTypeTag, t, 0}
}

func AtHash(h string) Ref {
	return &symbolicRef{RefTypeHash, h, 0}
}

type RefType int

func (t RefType) String() string {
	switch t {
	case RefTypeUnknown:
		return "unknown"
	case RefTypeHash:
		return "hash"
	case RefTypeBranch:
		return "branch"
	case RefTypeTag:
		return "tag"
	default:
		return fmt.Sprintf("<invalid: %d>", t)
	}
}

const (
	RefTypeUnknown RefType = iota
	RefTypeHash
	// A branch is generally a mutable
	RefTypeBranch
	RefTypeTag
)

type Ref interface {
	Resolve(RefResolver) (Hash, error)

	// TODO: Keep fmt.Stringer for debug printing, rename to Target() string?
	Target() string
	Type() RefType
	Before() uint8
}

func WithRef(ctx context.Context, s Ref) context.Context {
	if s == nil {
		return ctx
	}
	return context.WithValue(ctx, symbolicCtxKey, s)
}

func GetRef(ctx context.Context) (Ref, bool) {
	s, ok := ctx.Value(symbolicCtxKey).(Ref)
	return s, ok
}

type symbolicCtxKeyStruct struct{}

var symbolicCtxKey = symbolicCtxKeyStruct{}

type hash struct {
	hash    []byte
	encoded string
	src     Ref
}

func (h *hash) Hash() []byte   { return h.hash }
func (h *hash) String() string { return h.encoded }
func (h *hash) RefSource() Ref { return h.src }

type symbolicRef struct {
	st     RefType
	ref    string
	before uint8
}

func (r *symbolicRef) Target() string { return r.ref }
func (r *symbolicRef) Before() uint8  { return r.before }
func (r *symbolicRef) Type() RefType  { return r.st }
func (r *symbolicRef) Resolve(res RefResolver) (Hash, error) {
	// TODO: This is probably resolver-specific
	if r.before != 0 && r.st != RefTypeUnknown && r.st != RefTypeBranch {
		return nil, errors.New("setting Before() only works for branches")
	}
	return res.ResolveRef(r)
}

type MutableTarget interface {
	// The branch to which the resulting commit from the transaction
	// is added.
	DestBranch() string

	BaseCommit() Hash
	UUID() types.UID

	// TODO: Implement fmt.Stringer for debug printing
}

func NewMutableTarget(headBranch string, baseCommit Hash) MutableTarget {
	return &mutableTarget{headBranch: headBranch, baseCommit: baseCommit, uuid: uuid.NewUUID()}
}

type mutableTarget struct {
	headBranch string
	baseCommit Hash
	uuid       types.UID
}

func (m *mutableTarget) DestBranch() string { return m.headBranch }
func (m *mutableTarget) BaseCommit() Hash   { return m.baseCommit }
func (m *mutableTarget) UUID() types.UID    { return m.uuid }

func WithMutableTarget(ctx context.Context, m MutableTarget) context.Context {
	if m == nil {
		return ctx
	}
	return context.WithValue(ctx, mutableCtxKey, m)
}

func GetMutableTarget(ctx context.Context) (MutableTarget, bool) {
	mt, ok := ctx.Value(mutableCtxKey).(MutableTarget)
	return mt, ok
}

type mutableCtxKeyStruct struct{}

var mutableCtxKey = mutableCtxKeyStruct{}
