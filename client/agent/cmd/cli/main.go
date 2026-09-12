package main

import (
	"context"
	"fmt"
	"os"
	"time"

	sharediskv1 "github.com/share-disk/share-disk/contracts/proto/sharedisk/v1"

	"github.com/share-disk/share-disk/client/agent/internal/localapi"
	"github.com/share-disk/share-disk/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version":
		printVersion()
	case "help":
		printUsage()
	case "file":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Usage: share-disk-cli file <subcommand>\n")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "import":
			handleFileImport()
		case "stat":
			handleFileStat()
		case "list":
			handleLocalFileList(false)
		case "show":
			handleFileShow()
		case "export":
			handleFileExport()
		case "verify":
			handleFileVerify()
		case "rename":
			handleLocalFileMutation("rename")
		case "delete":
			handleLocalFileMutation("trash")
		default:
			fmt.Fprintf(os.Stderr, "Unknown file subcommand: %s\n", os.Args[2])
			os.Exit(1)
		}
	case "object":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Usage: share-disk-cli object <subcommand>\n")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "verify":
			handleObjectVerify()
		default:
			fmt.Fprintf(os.Stderr, "Unknown object subcommand: %s\n", os.Args[2])
			os.Exit(1)
		}
	case "status":
		handleStatus()
	case "setup", "login":
		handleCoordinatorEnroll()
	case "device":
		if len(os.Args) == 3 && os.Args[2] == "status" {
			handleStatus()
		} else {
			fmt.Fprintln(os.Stderr, "Usage: share-disk-cli device status")
			os.Exit(1)
		}
	case "transfer":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: share-disk-cli transfer <list|cancel>")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "list":
			handleTransferList()
		case "cancel":
			handleTransferCancel()
		default:
			fmt.Fprintf(os.Stderr, "Unknown transfer subcommand: %s\n", os.Args[2])
			os.Exit(1)
		}
	case "trash":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: share-disk-cli trash <list|restore|purge>")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "list":
			handleLocalFileList(true)
		case "restore":
			handleLocalFileMutation("restore")
		case "purge":
			handleLocalFileMutation("purge")
		default:
			fmt.Fprintf(os.Stderr, "Unknown trash subcommand: %s\n", os.Args[2])
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printVersion() {
	info := version.Get()
	fmt.Printf("share-disk CLI %s\n", info.Version)
	fmt.Printf("Commit: %s\n", info.Commit)
	fmt.Printf("Built: %s\n", info.BuildTime)
	fmt.Printf("Go: %s\n", info.GoVersion)
	fmt.Printf("OS/Arch: %s/%s\n", info.OS, info.Arch)
}

func printUsage() {
	fmt.Println("Usage: share-disk-cli <command>")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  version           Print version information")
	fmt.Println("  file import       Import a file into the managed object area (via the agent)")
	fmt.Println("  file stat         Show information about a local object (via the agent)")
	fmt.Println("  file list         List local active files belonging to the provisioned account")
	fmt.Println("  file show         Show a local file: file show <id>")
	fmt.Println("  file export       Export without overwrite: file export <id> <absolute-path>")
	fmt.Println("  file verify       Verify a local file by ID: file verify <id>")
	fmt.Println("  file rename       Rename a local file: file rename <id> <name>")
	fmt.Println("  file delete       Move a local file to trash: file delete <id>")
	fmt.Println("  trash list        List recoverable local files")
	fmt.Println("  trash restore     Restore a file: trash restore <id>")
	fmt.Println("  trash purge       Permanently delete a file: trash purge <id>")
	fmt.Println("  object verify     Verify an object's integrity (via the agent)")
	fmt.Println("  status            Show agent status (via the agent)")
	fmt.Println("  setup             Register/login this Agent: setup <account> (password via SHARE_DISK_PASSWORD)")
	fmt.Println("  login             Alias of setup")
	fmt.Println("  device status     Show local device and Agent status")
	fmt.Println("  transfer list     List locally observed server transfer tasks")
	fmt.Println("  transfer cancel   Request cancellation: transfer cancel <task-id>")
	fmt.Println("  help              Show this help message")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  SHARE_DISK_AGENT_SOCKET   Agent IPC socket path (default: /run/share-disk/agent.sock)")
	fmt.Println("  SHARE_DISK_PASSWORD       Account password used only by setup/login")
}

func handleCoordinatorEnroll() {
	account := argument(2, os.Args[1]+" <account>")
	password := os.Getenv("SHARE_DISK_PASSWORD")
	if password == "" {
		fatal(fmt.Errorf("SHARE_DISK_PASSWORD is required"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := agentClient().Call(ctx, &sharediskv1.LocalRequest{Payload: &sharediskv1.LocalRequest_CoordinatorEnroll{CoordinatorEnroll: &sharediskv1.CoordinatorEnrollRequest{Account: account, Password: password}}})
	if err != nil {
		fatal(err)
	}
	if err := responseError(resp); err != nil {
		fatal(err)
	}
	fmt.Println("Agent registered and logged in")
}

func handleFileShow() {
	file := findLocalFile(argument(3, "file show <file-id>"))
	fmt.Printf("ID: %s\nName: %s\nMIME: %s\nSize: %d bytes\nSHA-256: %s\nStatus: %s\n", file.GetId(), file.GetName(), file.GetMime(), file.GetSize(), file.GetSha256(), file.GetStatus())
}

func handleFileVerify() {
	file := findLocalFile(argument(3, "file verify <file-id>"))
	os.Args = []string{os.Args[0], "object", "verify", file.GetSha256()}
	handleObjectVerify()
}

func handleFileExport() {
	id := argument(3, "file export <file-id> <absolute-path>")
	destination := argument(4, "file export <file-id> <absolute-path>")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	resp, err := agentClient().Call(ctx, &sharediskv1.LocalRequest{Payload: &sharediskv1.LocalRequest_Export{Export: &sharediskv1.ExportRequest{FileId: id, DestinationPath: destination}}})
	if err != nil {
		fatal(err)
	}
	if err := responseError(resp); err != nil {
		fatal(err)
	}
	result := resp.GetExport()
	fmt.Printf("Exported: %s\nSize: %d bytes\nSHA-256: %s\n", result.GetDestinationPath(), result.GetSize(), result.GetSha256())
}

func findLocalFile(id string) *sharediskv1.LocalFileRecord {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, trash := range []bool{false, true} {
		resp, err := agentClient().Call(ctx, &sharediskv1.LocalRequest{Payload: &sharediskv1.LocalRequest_ListLocalFiles{ListLocalFiles: &sharediskv1.ListLocalFilesRequest{Trash: trash}}})
		if err != nil {
			fatal(err)
		}
		if err := responseError(resp); err != nil {
			fatal(err)
		}
		for _, file := range resp.GetListLocalFiles().GetFiles() {
			if file.GetId() == id {
				return file
			}
		}
	}
	fatal(fmt.Errorf("file not found: %s", id))
	return nil
}

func handleTransferList() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := agentClient().Call(ctx, &sharediskv1.LocalRequest{Payload: &sharediskv1.LocalRequest_ListTransfers{ListTransfers: &sharediskv1.ListTransfersRequest{Limit: 100}}})
	if err != nil {
		fatal(err)
	}
	if err := responseError(resp); err != nil {
		fatal(err)
	}
	for _, item := range resp.GetListTransfers().GetTransfers() {
		fmt.Printf("%s\t%s\tattempt=%d\t%s\n", item.GetId(), item.GetState(), item.GetAttempt(), item.GetObjectId())
	}
}

func handleTransferCancel() {
	id := argument(3, "transfer cancel <task-id>")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := agentClient().Call(ctx, &sharediskv1.LocalRequest{Payload: &sharediskv1.LocalRequest_CancelTransfer{CancelTransfer: &sharediskv1.CancelTransferRequest{TransferId: id}}})
	if err != nil {
		fatal(err)
	}
	if err := responseError(resp); err != nil {
		fatal(err)
	}
	if !resp.GetCancelTransfer().GetCanceled() {
		fatal(fmt.Errorf("transfer is not cancelable or was not found"))
	}
	fmt.Printf("Cancellation requested: %s\n", id)
}

func argument(index int, usage string) string {
	if len(os.Args) <= index || os.Args[index] == "" {
		fmt.Fprintf(os.Stderr, "Usage: share-disk-cli %s\n", usage)
		os.Exit(1)
	}
	return os.Args[index]
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}

func agentClient() *localapi.Client {
	socket := os.Getenv("SHARE_DISK_AGENT_SOCKET")
	if socket == "" {
		socket = "/run/share-disk/agent.sock"
	}
	return localapi.NewClient(socket)
}

func responseError(resp *sharediskv1.LocalResponse) error {
	if e := resp.GetError(); e != nil {
		return fmt.Errorf("%s: %s", e.GetCode(), e.GetMessage())
	}
	return nil
}

func handleFileImport() {
	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "Usage: share-disk-cli file import <path>\n")
		os.Exit(1)
	}
	sourcePath := os.Args[3]

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	resp, err := agentClient().Call(ctx, &sharediskv1.LocalRequest{
		Payload: &sharediskv1.LocalRequest_Import{
			Import: &sharediskv1.ImportRequest{SourcePath: sourcePath},
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := responseError(resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	imp := resp.GetImport()
	fmt.Printf("Imported object: %s\n", imp.GetFileObjectId())
	if imp.GetFileEntryId() != "" {
		fmt.Printf("Published file: %s\n", imp.GetFileEntryId())
	}
	fmt.Printf("Size: %d bytes\n", imp.GetSize())
}

func handleLocalFileList(trash bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := agentClient().Call(ctx, &sharediskv1.LocalRequest{Payload: &sharediskv1.LocalRequest_ListLocalFiles{ListLocalFiles: &sharediskv1.ListLocalFilesRequest{Trash: trash}}})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := responseError(resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	for _, file := range resp.GetListLocalFiles().GetFiles() {
		fmt.Printf("%s\t%s\t%d\t%s\t%s\n", file.GetId(), file.GetStatus(), file.GetSize(), file.GetSha256(), file.GetName())
	}
}

func handleLocalFileMutation(action string) {
	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "Usage: share-disk-cli %s %s <file-id>\n", os.Args[1], os.Args[2])
		os.Exit(1)
	}
	name := ""
	if action == "rename" {
		if len(os.Args) < 5 {
			fmt.Fprintln(os.Stderr, "Usage: share-disk-cli file rename <file-id> <name>")
			os.Exit(1)
		}
		name = os.Args[4]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := agentClient().Call(ctx, &sharediskv1.LocalRequest{Payload: &sharediskv1.LocalRequest_MutateLocalFile{MutateLocalFile: &sharediskv1.MutateLocalFileRequest{FileId: os.Args[3], Action: action, Name: name}}})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := responseError(resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	file := resp.GetMutateLocalFile().GetFile()
	if file == nil {
		fmt.Printf("%s completed: %s\n", action, os.Args[3])
		return
	}
	fmt.Printf("%s\t%s\t%s\n", file.GetId(), file.GetStatus(), file.GetName())
}

func handleFileStat() {
	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "Usage: share-disk-cli file stat <hash>\n")
		os.Exit(1)
	}
	identifier := os.Args[3]

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := agentClient().Call(ctx, &sharediskv1.LocalRequest{
		Payload: &sharediskv1.LocalRequest_Stat{
			Stat: &sharediskv1.StatRequest{Identifier: identifier},
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := responseError(resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	obj := resp.GetStat().GetObject()
	fmt.Printf("Object ID: %s\n", obj.GetId())
	fmt.Printf("Size: %d bytes\n", obj.GetSize())
	fmt.Printf("Chunk Size: %d\n", obj.GetChunkSize())
	fmt.Printf("Chunk Count: %d\n", obj.GetChunkCount())
	fmt.Printf("Status: %s\n", obj.GetStatus())
}

func handleObjectVerify() {
	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "Usage: share-disk-cli object verify <hash>\n")
		os.Exit(1)
	}
	hash := os.Args[3]

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	resp, err := agentClient().Call(ctx, &sharediskv1.LocalRequest{
		Payload: &sharediskv1.LocalRequest_Verify{
			Verify: &sharediskv1.VerifyRequest{ObjectId: hash},
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := responseError(resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	verify := resp.GetVerify()
	if verify.GetValid() {
		fmt.Println("Object is valid")
	} else {
		fmt.Printf("Verification failed: %s\n", verify.GetError())
		os.Exit(1)
	}
}

func handleStatus() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := agentClient().Call(ctx, &sharediskv1.LocalRequest{
		Payload: &sharediskv1.LocalRequest_Status{
			Status: &sharediskv1.StatusRequest{},
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := responseError(resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	st := resp.GetStatus()
	fmt.Printf("Agent Status\n")
	fmt.Printf("============\n")
	fmt.Printf("Version: %s\n", st.GetVersion())
	fmt.Printf("Device ID: %s\n", st.GetDeviceId())
	fmt.Printf("Storage Root: %s\n", st.GetStorageRoot())
	fmt.Printf("Objects: %d\n", st.GetObjectCount())
	fmt.Printf("Storage Used: %d bytes\n", st.GetStorageUsed())
	fmt.Printf("Storage Available: %d bytes\n", st.GetStorageAvailable())
}
