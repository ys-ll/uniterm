package session

// SPICEProxy bridges all channels of one SPICE client. target is either a TCP
// address or a legacy ws(s) endpoint; both use the same authenticated Web path.
type SPICEProxy struct{ *remoteDesktopProxy }

func NewSPICEProxy(target string) *SPICEProxy {
	return &SPICEProxy{newRemoteDesktopProxy(target, 32)}
}
