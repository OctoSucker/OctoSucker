package execmcp

import "testing"

func TestHostMntBindSource(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"/Users/a/b", "/host_mnt/Users/a/b"},
		{"/Volumes/Data/x", "/host_mnt/Volumes/Data/x"},
		{"/private/var/tmp/t", "/host_mnt/private/var/tmp/t"},
		{"/host_mnt/Users/a", "/host_mnt/Users/a"},
		{"/repo/agent/workspace", "/repo/agent/workspace"},
		{"/home/ubuntu/proj", "/home/ubuntu/proj"},
	}
	for _, tc := range tests {
		if got := hostMntBindSource(tc.in); got != tc.want {
			t.Errorf("hostMntBindSource(%q) = %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestBindSourceForNestedDockerRequiresExplicitEnv(t *testing.T) {
	t.Setenv("EXEC_NESTED_DOCKER_USE_HOST_MNT_PREFIX", "")
	if got := bindSourceForNestedDocker("/Users/x"); got != "/Users/x" {
		t.Fatalf("without flag: got %q", got)
	}
	t.Setenv("EXEC_NESTED_DOCKER_USE_HOST_MNT_PREFIX", "1")
	if got := bindSourceForNestedDocker("/Users/x"); got != "/host_mnt/Users/x" {
		t.Fatalf("with flag: got %q", got)
	}
}
