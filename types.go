package marzipan

const (
	Action              = "Action"
	CommandType         = "CommandType"
	MobileInternalIndex = "MobileInternalIndex"
)

type Request interface {
	SetMobileInternalIndex(string)
}

type Response interface {
	MobileInternalIndex() string
	CommandType() string
	Action() string
}

type Meta struct {
	MII string `json:"MobileInternalIndex,omitempty"`
	CT  string `json:"CommandType,omitempty"`
	ACT string `json:"Action,omitempty"`
}

func (m *Meta) MobileInternalIndex() string {
	return m.MII
}
func (m *Meta) SetMobileInternalIndex(mii string) {
	m.MII = mii
}

func (m *Meta) CommandType() string {
	return m.CT
}
func (m *Meta) SetCommandType(ct string) {
	m.CT = ct
}

func (m *Meta) Action() string {
	return m.ACT
}
func (m *Meta) SetAction(a string) {
	m.ACT = a
}

type DeviceList struct {
	Meta
	Devices map[string]Device
}

func NewDeviceListRequest() Request {
	return &Meta{
		CT: "DeviceList",
	}
}

type DynamicIndexUpdated struct {
	Meta
	Devices map[string]Device
}

type Device struct {
	Data         map[string]string
	DeviceValues map[string]DeviceValue
}

type DeviceValue struct {
	Name  string
	Value string
}
