package ops

import (
	"context"
	"encoding/json"

	"local-fx-host/internal/protocol"
)

// cancelArgs follows PROTOCOL.md §6. TargetID names the in-flight request
// whose handler should be canceled; a missing/empty targetId is a client
// bug and is rejected with E_BAD_REQUEST.
type cancelArgs struct {
	TargetID string `json:"targetId"`
}

// cancelData is the Response.Data shape. Accepted mirrors whether a
// matching job was found at cancel time — not whether the underlying op
// has actually wound down yet. Handlers emit their own "done" event with
// canceled=true once cleanup finishes.
type cancelData struct {
	Accepted bool `json:"accepted"`
}

// Cancel is the handler for the "cancel" op. It's intentionally a regular
// (non-streaming) handler so that it can be dispatched immediately without
// competing for the target op's goroutine. CancelJob is atomic: at most one
// Cancel request will see Accepted=true per job.
func Cancel(_ context.Context, req protocol.Request) protocol.Response {
	var args cancelArgs
	if len(req.Args) > 0 {
		if err := json.Unmarshal(req.Args, &args); err != nil {
			return protocol.ErrorResponse(req.ID, protocol.ErrCodeBadRequest,
				"invalid args: "+err.Error(), false)
		}
	}
	if args.TargetID == "" {
		return protocol.ErrorResponse(req.ID, protocol.ErrCodeBadRequest,
			"targetId required", false)
	}
	accepted := CancelJob(args.TargetID)
	return protocol.SuccessResponse(req.ID, cancelData{Accepted: accepted})
}
