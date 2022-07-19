// Package evtbus implements a local event bus. It provides methods to:
//
//  - publish events to the bus
//  - subscribe to events on the bus
//
// # Example
//
// 	type MyEvent struct {
// 		name string
// 	}
//
// 	type MySubscription struct{}
//
// 	func (*MySubscription) On(event *MyEvent) {
// 		// handle when a MyEvent has been published
// 	}
//
// 	func main() {
// 		var (
// 			bus   = evtbus.New()
// 			event = &MyEvent{"foo"}
// 			sub   = &MySubscription{}
// 		)
//
// 		evtbus.Sub[*MyEvent](bus, sub)
// 		bus.Publish(event)
// 	}
//
// # Publish Options
//
// ## Wait
//
// The Wait publish option will cause the call to Publish to wait for all event
// triggers to complete, even if there are async subscriptions.
//
//  bus.Publish(event, evtbus.Wait())
//
// # Subscription Options
//
// ## RunAsync
//
// The RunAync subscription option will cause a subscription to be triggered
// asyncronously. Meaning the On method will be call in a go routine.
//
//  evtbus.Sub[*MyEvent](bus, sub, evtbus.RunAsync())
package evtbus

import (
	"sync"
)

type (
	// An EventBus allows for publishing and subscribing to Events.
	EventBus interface {
		// Publish publishes an event to the event bus. Any matching
		// registered Subscription will be triggered.
		//
		// # Options
		//
		// ## Wait
		//
		// The Wait publish option will cause the call to Publish to wait for
		// all event triggers to complete, even if there are async
		// subscriptions.
		//
		//  bus.Publish(event, evtbus.Wait())
		Publish(event Event, options ...PublishOption)

		// Subscribe registers the subscription to be triggered when a matching
		// event is triggered.
		//
		// returns a SubscriptionHandle so that the subscription can be removed
		//
		// # Subscription Options
		//
		// ## RunAsync
		//
		// The RunAync subscription option will cause a subscription to be
		// triggered asyncronously. Meaning the On method will be call in a go
		// routine.
		//
		//  evtbus.Sub[*MyEvent](bus, sub, evtbus.RunAsync())
		Subscribe(
			subscriptions Subscription[Event],
			options ...SubscriptionOption,
		) SubscriptionHandle

		// Remove removes a subscription with the given handle. The handle is
		// obtained from the Subscribe call.
		//
		//  var handle SubscriptionHandle
		//  evtbus.Sub[*MyEvent](bus, sub)
		Remove(handle SubscriptionHandle)
	}

	// An Event is an object that can be published to an EventBus.
	Event interface{}

	// Subscription describes an On function that takes in an Event of type E.
	Subscription[E Event] interface {
		// On is called by an EventBus when a matching event of type E is
		// published to it
		On(event E)
	}

	// IEventBus implements [EventBus].
	//
	// IEventBus is safe to use from multiple go routines.
	IEventBus struct {
		subscriptionEntries    map[SubscriptionHandle]*subscriptionEntry
		nextSubscriptionHandle SubscriptionHandle
		lock                   sync.RWMutex
	}

	// TypedSubscription is an implementation of a Subscription that provides
	// an On function that accepts any Event type. When called, if the provided
	// Subscription that accepts Events of type E, it will call the On method
	// of it's given Subscription.
	TypedSubscription[E Event] struct {
		subscription Subscription[E]
	}

	// SubscriptionHandle is a unique value returned when Subscribe is called
	// on an EventBus. This handle can be used to remove the Subscription by
	// calling Remove on the EventBus.
	SubscriptionHandle uint

	// publishOptions holds the options for the EventBus Publish method call.
	publishOptions struct {
		wait bool
	}

	// PublishOption describes an option that can be used in an EventBus
	// Publish method.
	PublishOption interface {
		// isPublishOption returns true if it is a PublishOption
		isPublishOption() bool
	}

	// PublishOptionWait is an option that can be used in the EventBus Publish
	// method. When set to true, the Publish method will wait for all events
	// to be handled by all Subscriptions, even if the Subscriptions are Async.
	PublishOptionWait struct {
		// whether to wait for completion or not
		value bool
	}

	// SubsciptionOption is an optional argument that can be used in the
	// EventBus Subscribe call.
	SubscriptionOption interface {
		isSubscriptionOption() bool
	}

	// SubscriptionOptionExecutionStyle is an option that can be used in the
	// EventBus Subscribe method. It dictates whether the Subscription should
	// be executed synchonously or asyncronously (in a go routine).
	SubscriptionOptionExecutionStyle struct {
		value ExecutionStyle
	}

	// ExecutionStyle defines whether a process is executed synchronously or
	// asyncronously.
	ExecutionStyle uint8

	// subscriptionEntry is used to store Subscriptions along with their
	// optoins.
	subscriptionEntry struct {
		subscription   Subscription[Event]
		executionStyle ExecutionStyle
	}
)

const (
	Sync ExecutionStyle = iota
	Async
)

var (
	_ EventBus           = (*IEventBus)(nil)
	_ SubscriptionOption = (*SubscriptionOptionExecutionStyle)(
		nil,
	)
)

// New returns a new IEventBus object.
func New() *IEventBus {
	return &IEventBus{
		subscriptionEntries:    map[SubscriptionHandle]*subscriptionEntry{},
		nextSubscriptionHandle: 0,
		lock:                   sync.RWMutex{},
	}
}

func (bus *IEventBus) Publish(event Event, options ...PublishOption) {
	bus.lock.RLock()
	defer bus.lock.RUnlock()

	var (
		waitGroup      = &sync.WaitGroup{}
		publishOptions = newPublishOptions(options)
	)

	for _, entry := range bus.subscriptionEntries {
		entry.trigger(event, waitGroup)
	}

	if publishOptions.wait {
		waitGroup.Wait()
	}
}

func (bus *IEventBus) Subscribe(
	subscription Subscription[Event],
	options ...SubscriptionOption,
) SubscriptionHandle {
	bus.lock.Lock()
	defer bus.lock.Unlock()

	entry := &subscriptionEntry{
		subscription:   subscription,
		executionStyle: Sync,
	}

	for _, option := range options {
		option.isSubscriptionOption()

		if option, ok := option.(*SubscriptionOptionExecutionStyle); ok {
			entry.executionStyle = option.value
		}
	}

	handle := bus.getNextSubscriptionHandleUnsafe()

	bus.subscriptionEntries[handle] = entry

	return handle
}

func (bus *IEventBus) Remove(handle SubscriptionHandle) {
	bus.lock.Lock()
	defer bus.lock.Unlock()

	delete(bus.subscriptionEntries, handle)
}

func (bus *IEventBus) getNextSubscriptionHandleUnsafe() SubscriptionHandle {
	handle := bus.nextSubscriptionHandle
	bus.nextSubscriptionHandle++

	return handle
}

func newPublishOptions(options []PublishOption) *publishOptions {
	_publishOptions := &publishOptions{
		wait: false,
	}

	for _, option := range options {
		option.isPublishOption()

		if option, ok := option.(*PublishOptionWait); ok {
			_publishOptions.wait = option.value
		}
	}

	return _publishOptions
}

func (entry *subscriptionEntry) trigger(
	event Event,
	waitGroup *sync.WaitGroup,
) {
	switch entry.executionStyle {
	case Sync:
		entry.subscription.On(event)
	case Async:
		waitGroup.Add(1)

		runAsync := func() {
			defer waitGroup.Done()
			entry.subscription.On(event)
		}
		go runAsync()
	}
}

func Sub[E Event](
	bus EventBus,
	subscription Subscription[E],
	options ...SubscriptionOption,
) SubscriptionHandle {
	return bus.Subscribe(NewTypedSub(subscription), options...)
}

func NewTypedSub[E Event](subscription Subscription[E]) *TypedSubscription[E] {
	return &TypedSubscription[E]{
		subscription: subscription,
	}
}

func (sub *TypedSubscription[E]) On(event Event) {
	if event, ok := event.(E); ok {
		sub.subscription.On(event)
	}
}

func (*PublishOptionWait) isPublishOption() bool {
	return true
}

func Wait() *PublishOptionWait {
	return &PublishOptionWait{true}
}

func (*SubscriptionOptionExecutionStyle) isSubscriptionOption() bool {
	return true
}

func RunSync() *SubscriptionOptionExecutionStyle {
	return &SubscriptionOptionExecutionStyle{Sync}
}

func RunAsync() *SubscriptionOptionExecutionStyle {
	return &SubscriptionOptionExecutionStyle{Async}
}
