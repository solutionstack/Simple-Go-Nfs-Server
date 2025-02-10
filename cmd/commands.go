package cmd

import (
	"os"

	"github.com/alecthomas/kingpin/v2"
)

var (
	app           *kingpin.Application
	cliReplica    *kingpin.CmdClause
	cliMaster     *kingpin.CmdClause
	masterServer  *MasterNode
	replicaServer *ReplicaNode
)

func Execute() error {
	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	case cliMaster.FullCommand():

	case cliReplica.FullCommand():
	default:
	}
	return nil
}

func init() {
	masterServer = &MasterNode{}
	replicaServer = &ReplicaNode{}

	app = kingpin.New("", "Simple Network File System application in Go")
	app.HelpFlag.Hidden()

	cliMaster = app.Command("master", "run the nfs-server in master mode")
	cliMaster.Action(masterServer.Run)

	cliReplica = app.Command("replica", "run the nfs-server in replica mode")
	cliReplica.Action(replicaServer.Run)

	cliReplica.Flag("port", "port this replica node would listen on").Required().Short('P').Uint16Var(&replicaServer.Port)
	cliReplica.Flag("storage-root", "relative path from server root, where files are stored").Short('r').PlaceHolder("/path/to/storage-root").StringVar(&replicaServer.StoragePath)
	cliMaster.Flag("storage-root", "relative path from server root, where files are stored").Required().Short('r').PlaceHolder("/path/to/storage-root").StringVar(&masterServer.StoragePath)
	cliMaster.Flag("http-port", "port for the master nfs api server").Required().Short('P').Uint16Var(&masterServer.HTTPPort)
	cliMaster.Flag("peers", "comma seperated list of replica servers:port instances").Short('p').
		PlaceHolder("0.0.0.0:1111,6.7.8.9:9999").Required().Strings()

	cliMaster.Validate(ParseReplicaPeerList)

	kingpin.CommandLine.Help = ""
	kingpin.UsageTemplate(kingpin.DefaultUsageTemplate).Version("0.0.1").Author("Olubodun Agbalaya")

}
