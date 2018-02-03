package matcher

type (
	addParticipantResponse struct {
		participant *SessionParticipant
		session     *Session
		err         error
	}
)

type (
	addParticipantRequest struct {
		participant *Participant
		resp        chan addParticipantResponse
	}
)
