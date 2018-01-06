// Code generated by mockery v1.0.1 DO NOT EDIT.

package nlmock

import mock "github.com/stretchr/testify/mock"
import net "net"
import netns "github.com/vishvananda/netns"
import nl "github.com/kelda/kelda/minion/nl"

// I is an autogenerated mock type for the I type
type I struct {
	mock.Mock
}

// AddVeth provides a mock function with given fields: name, peer, mtu
func (_m *I) AddVeth(name string, peer string, mtu int) error {
	ret := _m.Called(name, peer, mtu)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, int) error); ok {
		r0 = rf(name, peer, mtu)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// AddrAdd provides a mock function with given fields: link, ip
func (_m *I) AddrAdd(link nl.Link, ip net.IPNet) error {
	ret := _m.Called(link, ip)

	var r0 error
	if rf, ok := ret.Get(0).(func(nl.Link, net.IPNet) error); ok {
		r0 = rf(link, ip)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// CloseNsHandle provides a mock function with given fields: ns
func (_m *I) CloseNsHandle(ns netns.NsHandle) error {
	ret := _m.Called(ns)

	var r0 error
	if rf, ok := ret.Get(0).(func(netns.NsHandle) error); ok {
		r0 = rf(ns)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// GetNetns provides a mock function with given fields:
func (_m *I) GetNetns() (netns.NsHandle, error) {
	ret := _m.Called()

	var r0 netns.NsHandle
	if rf, ok := ret.Get(0).(func() netns.NsHandle); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(netns.NsHandle)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetNetnsFromPath provides a mock function with given fields: _a0
func (_m *I) GetNetnsFromPath(_a0 string) (netns.NsHandle, error) {
	ret := _m.Called(_a0)

	var r0 netns.NsHandle
	if rf, ok := ret.Get(0).(func(string) netns.NsHandle); ok {
		r0 = rf(_a0)
	} else {
		r0 = ret.Get(0).(netns.NsHandle)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(_a0)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// LinkByIndex provides a mock function with given fields: index
func (_m *I) LinkByIndex(index int) (nl.Link, error) {
	ret := _m.Called(index)

	var r0 nl.Link
	if rf, ok := ret.Get(0).(func(int) nl.Link); ok {
		r0 = rf(index)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(nl.Link)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(int) error); ok {
		r1 = rf(index)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// LinkByName provides a mock function with given fields: name
func (_m *I) LinkByName(name string) (nl.Link, error) {
	ret := _m.Called(name)

	var r0 nl.Link
	if rf, ok := ret.Get(0).(func(string) nl.Link); ok {
		r0 = rf(name)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(nl.Link)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(name)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// LinkDel provides a mock function with given fields: link
func (_m *I) LinkDel(link nl.Link) error {
	ret := _m.Called(link)

	var r0 error
	if rf, ok := ret.Get(0).(func(nl.Link) error); ok {
		r0 = rf(link)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// LinkSetHardwareAddr provides a mock function with given fields: link, hwaddr
func (_m *I) LinkSetHardwareAddr(link nl.Link, hwaddr net.HardwareAddr) error {
	ret := _m.Called(link, hwaddr)

	var r0 error
	if rf, ok := ret.Get(0).(func(nl.Link, net.HardwareAddr) error); ok {
		r0 = rf(link, hwaddr)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// LinkSetName provides a mock function with given fields: link, name
func (_m *I) LinkSetName(link nl.Link, name string) error {
	ret := _m.Called(link, name)

	var r0 error
	if rf, ok := ret.Get(0).(func(nl.Link, string) error); ok {
		r0 = rf(link, name)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// LinkSetNs provides a mock function with given fields: link, nsh
func (_m *I) LinkSetNs(link nl.Link, nsh netns.NsHandle) error {
	ret := _m.Called(link, nsh)

	var r0 error
	if rf, ok := ret.Get(0).(func(nl.Link, netns.NsHandle) error); ok {
		r0 = rf(link, nsh)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// LinkSetUp provides a mock function with given fields: link
func (_m *I) LinkSetUp(link nl.Link) error {
	ret := _m.Called(link)

	var r0 error
	if rf, ok := ret.Get(0).(func(nl.Link) error); ok {
		r0 = rf(link)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// RouteAdd provides a mock function with given fields: r
func (_m *I) RouteAdd(r nl.Route) error {
	ret := _m.Called(r)

	var r0 error
	if rf, ok := ret.Get(0).(func(nl.Route) error); ok {
		r0 = rf(r)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// RouteList provides a mock function with given fields: family
func (_m *I) RouteList(family int) ([]nl.Route, error) {
	ret := _m.Called(family)

	var r0 []nl.Route
	if rf, ok := ret.Get(0).(func(int) []nl.Route); ok {
		r0 = rf(family)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]nl.Route)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(int) error); ok {
		r1 = rf(family)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// SetNetns provides a mock function with given fields: ns
func (_m *I) SetNetns(ns netns.NsHandle) error {
	ret := _m.Called(ns)

	var r0 error
	if rf, ok := ret.Get(0).(func(netns.NsHandle) error); ok {
		r0 = rf(ns)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}
