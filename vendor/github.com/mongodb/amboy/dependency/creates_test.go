package dependency

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

// CreatesFileSuite tests the dependency.Manager
// implementation that checks for the existence of a file. If the file
// exist the dependency becomes a noop.
type CreatesFileSuite struct {
	dep      *CreatesFile
	packages []string
	suite.Suite
}

func TestCreatesFileSuite(t *testing.T) {
	suite.Run(t, new(CreatesFileSuite))
}

func (s *CreatesFileSuite) SetupSuite() {
	s.packages = []string{"job", "dependency", "queue", "pool", "build", "registry"}
}

func (s *CreatesFileSuite) SetupTest() {
	s.dep = NewCreatesFileInstance()
}

func (s *CreatesFileSuite) TestInstanceImplementsManagerInterface() {
	s.Implements((*Manager)(nil), s.dep)
}

func (s *CreatesFileSuite) TestConstructorCreatesObjectWithFileNameSet() {
	for _, dir := range s.packages {
		dep := NewCreatesFile(dir)
		s.Equal(dir, dep.FileName)
	}
}

func (s *CreatesFileSuite) TestDependencyWithoutFileSetReportsReady() {
	s.Equal(s.dep.FileName, "")
	s.Equal(s.dep.State(), Ready)

	s.dep.FileName = " \\[  ]"
	s.Equal(s.dep.State(), Ready)

	s.dep.FileName = "foo"
	s.Equal(s.dep.State(), Ready)

	s.dep.FileName = " "
	s.Equal(s.dep.State(), Ready)

	s.dep.FileName = ""
	s.Equal(s.dep.State(), Ready)
}

func (s *CreatesFileSuite) TestAmboyPackageDirectoriesExistAndReportPassedState() {
	for _, dir := range s.packages {
		dep := NewCreatesFile("../" + dir)
		s.Equal(dep.State(), Passed, dir)
	}

}

func (s *CreatesFileSuite) TestCreatesDependencyTestReportsExpectedType() {
	t := s.dep.Type()
	s.Equal(t.Name, "create-file")
	s.Equal(t.Version, 0)
}
