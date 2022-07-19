package evtbus_test

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/yo3jones/goevtbus/pkg/evtbus"
)

type (
	callTracker interface {
		getCalls() []event
	}

	subscription[E event] struct {
		calls []event
	}

	event interface {
		Name() string
	}

	iEvent struct {
		name string
	}

	subEvent struct {
		name string
	}

	publishSubscribeTestCase struct {
		bus                 evtbus.EventBus
		tracker             callTracker
		name                string
		expectOnCallNames   []string
		events              []event
		publishOptions      []evtbus.PublishOption
		subscriptionOptions []evtbus.SubscriptionOption
		useSubEvent         bool
		sortOutput          bool
		handle              evtbus.SubscriptionHandle
	}
)

const (
	bar = "bar"
	buz = "buz"
	fiz = "fiz"
	foo = "foo"
)

func newSubscription[E event]() *subscription[E] {
	return &subscription[E]{
		calls: []event{},
	}
}

func (sub *subscription[E]) On(event E) {
	sub.calls = append(sub.calls, event)
}

func (sub *subscription[E]) getCalls() []event {
	return sub.calls
}

func (event *iEvent) Name() string {
	return event.name
}

func (event *subEvent) Name() string {
	return event.name
}

func (test *publishSubscribeTestCase) setup() {
	test.bus = evtbus.New()

	if test.useSubEvent {
		sub := newSubscription[*subEvent]()
		test.tracker = sub
		test.handle = evtbus.Sub[*subEvent](
			test.bus,
			sub,
			test.subscriptionOptions...)
	} else {
		sub := newSubscription[event]()
		test.tracker = sub
		test.handle = evtbus.Sub[event](
			test.bus,
			sub,
			test.subscriptionOptions...,
		)
	}
}

func (test *publishSubscribeTestCase) publishAll() {
	for _, event := range test.events {
		test.bus.Publish(event, test.publishOptions...)
	}
}

func (test *publishSubscribeTestCase) getCallNames() []string {
	names := []string{}
	for _, event := range test.tracker.getCalls() {
		names = append(names, event.Name())
	}

	if test.sortOutput {
		sort.Strings(names)
	}

	return names
}

func (test *publishSubscribeTestCase) remove() {
	test.bus.Remove(test.handle)
}

func getPublishSubscribeTestCases() []publishSubscribeTestCase {
	return []publishSubscribeTestCase{
		{
			name: "with simple",
			events: []event{
				&iEvent{foo},
				&subEvent{bar},
			},
			expectOnCallNames: []string{
				foo,
				bar,
			},
		},
		{
			name: "with sub event subscription",
			subscriptionOptions: []evtbus.SubscriptionOption{
				evtbus.RunSync(),
			},
			useSubEvent: true,
			events: []event{
				&iEvent{fiz},
				&subEvent{foo},
				&subEvent{bar},
				&iEvent{buz},
			},
			expectOnCallNames: []string{
				foo,
				bar,
			},
		},
		{
			name:       "with async option",
			sortOutput: true,
			publishOptions: []evtbus.PublishOption{
				evtbus.Wait(),
			},
			subscriptionOptions: []evtbus.SubscriptionOption{
				evtbus.RunAsync(),
			},
			events: []event{
				&iEvent{bar},
				&iEvent{foo},
			},
			expectOnCallNames: []string{
				bar,
				foo,
			},
		},
	}
}

func TestPublishSubscribe(t *testing.T) {
	t.Parallel()

	for _, tc := range getPublishSubscribeTestCases() {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var gotOnCallNames []string

			tc.setup()
			tc.publishAll()
			tc.remove()
			tc.publishAll()

			gotOnCallNames = tc.getCallNames()

			if !reflect.DeepEqual(gotOnCallNames, tc.expectOnCallNames) {
				t.Errorf(
					strings.Join([]string{
						"expected subscription to be called with \n%v\n but",
						"got \n%v\n",
					}, " "),
					tc.expectOnCallNames,
					gotOnCallNames,
				)
			}
		})
	}
}
