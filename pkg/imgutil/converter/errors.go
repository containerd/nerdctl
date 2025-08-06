package converter

import (
	"errors"
)

var (
	ErrZstdInRequiresExperimental   = errors.New("option --zstdchunked-record-in requires experimental mode to be enabled")
	ErrESGZTocRequiresExperimental  = errors.New("option --estargz-external-toc requires experimental mode to be enabled")
	ErrESGZDiffRequiresExperimental = errors.New("option --estargz-keep-diff-id requires experimental mode to be enabled")
	ErrESGSInRequiresExperimental   = errors.New("option --estargz-record-in requires experimental mode to be enabled")
)
