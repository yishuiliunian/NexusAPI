package errors

import (
	stderrors "errors"
	"testing"
)

func TestNew_FormatError(t *testing.T) {
	e := New(CodeNotFound, "not here")
	if e.Error() != "not_found: not here" {
		t.Errorf("got %q", e.Error())
	}
	if e.Code() != CodeNotFound || e.Message() != "not here" {
		t.Errorf("code/msg mismatch: %+v", e)
	}
}

func TestWrap_IncludesCause(t *testing.T) {
	cause := stderrors.New("boom")
	e := Wrap(CodeInternal, "save failed", cause)
	if e.Error() != "internal: save failed: boom" {
		t.Errorf("got %q", e.Error())
	}
	if !stderrors.Is(e, cause) {
		t.Error("Unwrap should expose cause")
	}
}

func TestIs_MatchesCode(t *testing.T) {
	e := New(CodeInvalidArgument, "bad")
	if !Is(e, CodeInvalidArgument) {
		t.Error("Is should match same code")
	}
	if Is(e, CodeNotFound) {
		t.Error("Is should reject different code")
	}
	if Is(stderrors.New("plain"), CodeInvalidArgument) {
		t.Error("Is should reject non-Error")
	}
}

func TestIs_WrappedError(t *testing.T) {
	inner := New(CodeRateLimited, "slow down")
	wrapped := Wrap(CodeInternal, "ctx", inner)
	// Is compares top-level Code only; wrapped outer should win.
	if !Is(wrapped, CodeInternal) {
		t.Error("want CodeInternal on top-level")
	}
}

func TestHTTPStatus_Mapping(t *testing.T) {
	cases := map[Code]int{
		CodeInvalidArgument:   400,
		CodeUnauthenticated:   401,
		CodePermissionDenied:  403,
		CodeNotFound:          404,
		CodeAlreadyExists:     409,
		CodeRateLimited:       429,
		CodeInsufficientQuota: 402,
		CodeTimeout:           504,
		CodeUpstream:          502,
		CodeInternal:          500,
		Code("unknown"):       500,
	}
	for code, want := range cases {
		if got := HTTPStatus(code); got != want {
			t.Errorf("%s → %d, want %d", code, got, want)
		}
	}
}

func TestSentinels_Reusable(t *testing.T) {
	if ErrNotFound.Code() != CodeNotFound {
		t.Error("ErrNotFound code mismatch")
	}
	if !Is(ErrInsufficientQuota, CodeInsufficientQuota) {
		t.Error("sentinel not recognized by Is")
	}
}
