package features

import "errors"

var (
	ErrIPFSSupportMissing      = errors.New("ipfs support has been disabled by the distributor of this build")
	ErrESGZSupportMissing      = errors.New("estargz support has been disabled by the distributor of this build (eg: build tag `no_esgz`)")
	ErrOverlayBDSupportMissing = errors.New("overlaybd support has been disabled by the distributor of this build (eg: build tag `no_obd`)")
	ErrNydusSupportMissing     = errors.New("nydus support has been disabled by the distributor of this build (eg: build tag `no_nydus`)")
)
