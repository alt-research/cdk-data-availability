// Code generated by mockery v2.20.0. DO NOT EDIT.

package mocks

import (
	context "context"

	interfaces "github.com/0xPolygon/cdk-data-availability/types/interfaces"
	mock "github.com/stretchr/testify/mock"
)

// IEthClientFactory is an autogenerated mock type for the IEthClientFactory type
type IEthClientFactory struct {
	mock.Mock
}

// CreateEthClient provides a mock function with given fields: ctx, url
func (_m *IEthClientFactory) CreateEthClient(ctx context.Context, url string) (interfaces.IEthClient, error) {
	ret := _m.Called(ctx, url)

	var r0 interfaces.IEthClient
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (interfaces.IEthClient, error)); ok {
		return rf(ctx, url)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) interfaces.IEthClient); ok {
		r0 = rf(ctx, url)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(interfaces.IEthClient)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, url)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

type mockConstructorTestingTNewIEthClientFactory interface {
	mock.TestingT
	Cleanup(func())
}

// NewIEthClientFactory creates a new instance of IEthClientFactory. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewIEthClientFactory(t mockConstructorTestingTNewIEthClientFactory) *IEthClientFactory {
	mock := &IEthClientFactory{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
