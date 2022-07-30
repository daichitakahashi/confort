package beaconutil

const (
	ResourcePolicyError    = "error"
	ResourcePolicyReuse    = "reuse"
	ResourcePolicyTakeOver = "takeover"
)

func ValidResourcePolicy(s string) bool {
	switch s {
	case ResourcePolicyError, ResourcePolicyReuse, ResourcePolicyTakeOver:
		return true
	default:
		return false
	}
}
