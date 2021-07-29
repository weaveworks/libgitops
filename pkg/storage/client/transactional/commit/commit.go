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

type Commit interface {
	Hash() Hash
	Author() Signature
	Message() Message
	Parents() []Hash
}

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

type Hash interface {
	Hash() []byte
	String() string
}

func WithHash(ctx context.Context, h Hash) context.Context {
	if h == nil {
		return ctx
	}
	return context.WithValue(ctx, hashCtxKey, h)
}

func GetHash(ctx context.Context) Hash {
	return ctx.Value(hashCtxKey).(Hash)
}

type hashCtxKeyStruct struct{}

var hashCtxKey = hashCtxKeyStruct{}

type Ref interface {
	Resolve(RefResolver) (Hash, error)
}

type RefResolver interface {
	ResolveSymbolic(SymbolicRef) (Hash, error)
}

type Resolver interface {
	ResolveHash(Hash) (Commit, error)
}

func SHA1(h [20]byte) Hash {
	b := make([]byte, 20)
	copy(b, h[:])
	return &hash{hash: b, encoded: hex.EncodeToString(b)}
}

func FromSHA1(hash string) Ref {
	return &sha1Ref{ref: hash}
}

func At(symbolic string) SymbolicRef {
	return &symbolicRef{SymbolicTypeUnknown, symbolic, 0}
}

func Default() SymbolicRef {
	return AtBranch("") // Signifies the default branch
}

func AtBranch(b string) SymbolicRef {
	return Before(b, 0)
}

func Before(b string, n uint8) SymbolicRef {
	return &symbolicRef{SymbolicTypeBranch, b, n}
}

func AtTag(t string) SymbolicRef {
	return &symbolicRef{SymbolicTypeTag, t, 0}
}

func AtHash(h string) SymbolicRef {
	return &symbolicRef{SymbolicTypeHash, h, 0}
}

type SymbolicType int

const (
	SymbolicTypeUnknown SymbolicType = iota
	SymbolicTypeHash
	// A branch is generally a mutable
	SymbolicTypeBranch
	SymbolicTypeTag
)

type SymbolicRef interface {
	Ref

	String() string
	Index() uint8
	Type() SymbolicType
}

type hash struct {
	hash    []byte
	encoded string
}

func (h *hash) Hash() []byte   { return h.hash }
func (h *hash) String() string { return h.encoded }

type sha1Ref struct {
	ref string
}

func (r *sha1Ref) Resolve(RefResolver) (Hash, error) {
	b, err := hex.DecodeString(r.ref)
	if err != nil {
		return nil, err
	}
	return &hash{hash: b, encoded: r.ref}, nil
}

type symbolicRef struct {
	st    SymbolicType
	ref   string
	index uint8
}

func (r *symbolicRef) String() string     { return r.ref }
func (r *symbolicRef) Index() uint8       { return r.index }
func (r *symbolicRef) Type() SymbolicType { return r.st }
func (r *symbolicRef) Resolve(res RefResolver) (Hash, error) {
	// This is probably resolver-specific
	if r.index != 0 && r.st != SymbolicTypeUnknown && r.st != SymbolicTypeBranch {
		return nil, errors.New("index only works for branches")
	}
	return res.ResolveSymbolic(r)
}

type MutableTarget interface {
	HeadBranch() string
	BaseCommit() Hash
	UUID() types.UID
}

func NewMutableTarget(headBranch string, baseCommit Hash) MutableTarget {
	return &mutableTarget{headBranch: headBranch, baseCommit: baseCommit, uuid: uuid.New()}
}

type mutableTarget struct {
	headBranch string
	baseCommit Hash
	uuid       types.UID
}

func (m *mutableTarget) HeadBranch() string { return m.headBranch }
func (m *mutableTarget) BaseCommit() Hash   { return m.baseCommit }
func (m *mutableTarget) UUID() types.UID    { return m.uuid }

func WithMutableTarget(ctx context.Context, m MutableTarget) context.Context {
	if m == nil {
		return ctx
	}
	return context.WithValue(ctx, mutableCtxKey, m)
}

func GetMutableTarget(ctx context.Context) MutableTarget {
	return ctx.Value(mutableCtxKey).(MutableTarget)
}

type mutableCtxKeyStruct struct{}

var mutableCtxKey = mutableCtxKeyStruct{}
