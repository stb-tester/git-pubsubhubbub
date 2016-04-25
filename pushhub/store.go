package pushhub

import (
	"errors"
)

type Store interface {
	Subscribe(sub []Subscription) error
	Unsubscribe(sub []Subscription) error
	Load(cb func (*Subscription)) error
}

type NullStore struct {
};
func (s NullStore) Subscribe(sub []Subscription) error {
	return nil;
}
func (s NullStore) Unsubscribe(sub []Subscription) error {
	return nil;
}
func (s NullStore) Load(cb func (*Subscription)) error {
	return nil;
}

type JsonStore struct {
	filename string;
	subscriptions []Subscription;
}
func (s JsonStore) Subscribe(sub []Subscription) error {
	return errors.New("Not implemented");
}

func (s JsonStore) Unsubscribe(sub []Subscription) error {
	return errors.New("Not implemented");
}

func (s JsonStore) Load(cb func (*Subscription)) error {
	return errors.New("Not implemented");
}
