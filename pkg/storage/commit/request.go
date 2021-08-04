package commit

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

var _ Request = GenericRequest{}

type GenericRequest struct {
	Name        string
	Email       string
	When        *time.Time
	Title       string
	Description string
}

func (r GenericRequest) Author() Signature {
	return &signature{&r.Name, &r.Email, r.When}
}
func (r GenericRequest) Message() Message {
	return &message{&r.Title, &r.Description}
}
func (r GenericRequest) Validate() error {
	root := field.NewPath("commit.GenericRequest")
	allErrs := field.ErrorList{}
	if len(r.Name) == 0 {
		allErrs = append(allErrs, field.Required(root.Child("Name"), validation.EmptyError()))
	}
	// TODO: Should this be optional or not?
	if len(r.Email) == 0 {
		allErrs = append(allErrs, field.Required(root.Child("Email"), validation.EmptyError()))
	}
	if len(r.Title) == 0 {
		allErrs = append(allErrs, field.Required(root.Child("Title"), validation.EmptyError()))
	}
	return allErrs.ToAggregate()
}

type signature struct {
	name, email *string
	when        *time.Time
}

func (s *signature) Name() string     { return *s.name }
func (s *signature) Email() string    { return *s.email }
func (s *signature) When() *time.Time { return s.when }
func (s *signature) String() string   { return fmt.Sprintf("%s <%s>", s.Name(), s.Email()) }

type message struct {
	title, desc *string
}

func (m *message) Title() string       { return *m.title }
func (m *message) Description() string { return *m.desc }
func (m *message) String() string      { return fmt.Sprintf("%s\n\n%s", m.Title(), m.Description()) }
