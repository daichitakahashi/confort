package beacon

const (
	ResourcePolicyError    = "error"
	ResourcePolicyReuse    = "reuse"
	ResourcePolicyReusable = "reusable"
	ResourcePolicyTakeOver = "takeover"
)

func ValidResourcePolicy(s string) bool {
	switch s {
	case ResourcePolicyError, ResourcePolicyReuse, ResourcePolicyReusable, ResourcePolicyTakeOver:
		return true
	default:
		return false
	}
}
