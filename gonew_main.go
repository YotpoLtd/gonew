// Copyright 2011, Bryan Matsuo. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package main
/* 
*  File: gonew_main.go
*  Author: Bryan Matsuo [bmatsuo@soe.ucsc.edu] 
*  Created: Sat Jul  2 19:17:53 PDT 2011
*  Usage: gonew [options]
 */
import (
    "os"
    //"io"
    "log"
    "fmt"
    "flag"
    "strings"
    "template"
    //"bufio"
    //"io/ioutil"
    "path/filepath"
    //"github.com/hoisie/mustache.go"
    //"github.com/kr/pretty.go"
)

func ArgumentError(msg string) {
    fmt.Fprintf(os.Stderr, "%s\n", msg)
    fs := setupFlags()
    fs.Usage()
}

var (
    usage          = `
gonew [options] cmd NAME
gonew [options] pkg NAME
gonew [options] lib NAME PKG
`
    printUsageHead = func() { fmt.Fprint(os.Stderr, usage, "\n") }
    userepo        = false
    norepo         = true
    VERBOSE        = false
    DEBUG          = false
    DEBUG_LEVEL    = -1
    name           string
    ptype          string
    repo           string
    host           string
    user           string
    target         string
    license        string
    remote         string
    help           bool
)

func Debug(level int, msg string) {
    if DEBUG && DEBUG_LEVEL >= level {
        log.Print(msg)
    }
}

func Verbose(msg string) {
    if DEBUG || VERBOSE {
        fmt.Print(msg)
    }
}

func setupFlags() *flag.FlagSet {
    var fs = flag.NewFlagSet("gonew", flag.ExitOnError)
    fs.StringVar(&repo,
        "repo", "", "Repository type (e.g. 'git').")
    fs.StringVar(&host,
        "host", "", "Repository host if any (e.g. 'github').")
    fs.StringVar(&user,
        "user", "", "Repo host username.")
    fs.StringVar(&remote,
        "remote", "", "Remote repository url to initialize and push to.")
    fs.StringVar(&target,
        "target", "", "Makefile target. Default based on NAME.")
    fs.StringVar(&license,
        "license", "", "Project license (e.g. 'newbsd').")
    fs.BoolVar(&(AppConfig.MakeTest),
        "test", AppConfig.MakeTest, "Produce test files with Go files.")
    fs.BoolVar(&(norepo), "norepo", false, "Don't start a repository.")
    fs.BoolVar(&VERBOSE,
        "v", false, "Verbose output.")
    fs.IntVar(&DEBUG_LEVEL,
        "debug", -1, "Change the amout of debug output.")
    fs.BoolVar(&help,
        "help", false, "Show this message.")
    fs.Usage = func() {
        printUsageHead()
        fs.PrintDefaults()
    }
    return fs
}

type Request int

const (
    NilRequest Request = iota
    ProjectRequest
    LibraryRequest
)


var RequestedFile File
var RequestedProject Project

func parseArgs() Request {
    var fs = setupFlags()
    fs.Parse(os.Args[1:])
    userepo = !norepo
    if DEBUG_LEVEL >= 0 {
        DEBUG = true
    }
    if help {
        fs.Usage()
        os.Exit(0)
    }
    var narg = fs.NArg()
    if narg < 1 {
        ArgumentError("Missing TYPE argument")
        os.Exit(1)
    }
    if narg < 2 {
        ArgumentError("Missing NAME argument")
        os.Exit(1)
    }
    ptype = fs.Arg(0)
    name = fs.Arg(1)

    if target == "" {
        target = DefaultTarget(name)
    }
    var (
        file = File{
            Name: name, Pkg: "main",
            Repo: AppConfig.Repo, License: AppConfig.License,
            User: AppConfig.HostUser, Host: AppConfig.Host}
        project = Project{
            Name: name, Target: target,
            Type: NilProjectType, License: AppConfig.License, Remote: remote,
            Host: AppConfig.Host, User: AppConfig.HostUser,
            Repo: AppConfig.Repo}
        produceProject = true
        licObj         = NilLicenseType
        repoObj        = NilRepoType
        hostObj        = NilRepoHost
    )
    switch ptype {
    case "cmd":
        project.Type = CmdType
    case "pkg":
        project.Type = PkgType
    case "lib":
        produceProject = false
    default:
        fmt.Fprintf(os.Stderr, "Unknown TYPE %s\n", ptype)
        os.Exit(1)
    }
    switch license {
    case "":
        break
    case "newbsd":
        licObj = NewBSD
    default:
        fmt.Fprintf(os.Stderr, "Unknown TYPE %s\n", ptype)
        os.Exit(1)
    }
    if userepo {
        switch repo {
        case "":
            break
        case "git":
            repoObj = GitType
        case "mercurial":
            repoObj = HgType
        default:
            fmt.Fprintf(os.Stderr, "Unknown REPO %s\n", repo)
            os.Exit(1)
        }
        switch host {
        case "":
            break
        case "github":
            hostObj = GitHubHost
            repoObj = GitType
        /*
        case "googlecode":
            hostObj = GoogleCodeType
            repoObj = HgType
        */
        default:
            fmt.Fprintf(os.Stderr, "Unknown HOST %s\n", host)
            os.Exit(1)
        }
    }
    if produceProject {
        // TODO check target for improper characters.
        if user != "" {
            project.User = user
        }
        if licObj != NilLicenseType {
            project.License = licObj
        }
        if hostObj != NilRepoHost {
            project.Host = hostObj
        }
        if userepo && repoObj != NilRepoType {
            project.Repo = repoObj
        }
        RequestedProject = project
        return ProjectRequest
    } else {
        if narg < 3 {
            ArgumentError("Missing PKG argument")
            os.Exit(1)
        }
        file.Pkg = fs.Arg(2)
        if user != "" {
            file.User = user
        }
        if licObj != NilLicenseType {
            file.License = licObj
        }
        if hostObj != NilRepoHost {
            file.Host = hostObj
        }
        if userepo && repoObj != NilRepoType {
            file.Repo = repoObj
        }
        RequestedFile = file
        return LibraryRequest
    }
    return NilRequest
}


func FindTemplates() (TemplateMultiSet, os.Error) {
    troots := make([]string, 0, 2)
    if alt := AppConfig.AltRoot; alt != "" {
        fmt.Println(alt)
        troots = append(troots, alt)
    }
    if false {
        troots = append(troots, filepath.Join(GetTemplateRoot()...))
    }
    f := template.FuncMap{
        "import":func(pkg... string)string {
            if len(pkg) == 0 {
                return "import ()"
            }
            pieces := make([]string, len(pkg) + 2)
            pieces[0] = "import ("
            copy(pieces[1:], pkg)
            pieces[len(pieces)-1] = ")"
            return strings.Join(pieces, "\n")
        },
        "date":DateString,
        "year":YearString,
    }
    return MakeTemplateMultiSet(f, troots...)
}

func main() {
    if err := TouchConfig(); err != nil {
        fmt.Fprint(os.Stderr, err.String(), "\n")
        os.Exit(1)
    }
    Verbose("Parsing config file.\n")
    ReadConfig()

    ms, err := FindTemplates()
    if err != nil {
        panic(err)
    }
    if p, err := ExecutedSet(ms, "go.cmd.t", nil); err != nil {
        panic(err)
    } else {
        fmt.Println(string(p))
    }

    switch request := parseArgs(); request {
    case ProjectRequest:
        if DEBUG {
            fmt.Printf("Project requested %v\n", RequestedProject)
        } else if VERBOSE {
            fmt.Printf("Generating project %s\n", RequestedProject.Name)
        }
        if err := RequestedProject.Create(); err != nil {
            fmt.Fprint(os.Stderr, err.String(), "\n")
            os.Exit(1)
        }
    case LibraryRequest:
        if DEBUG {
            fmt.Printf("Library requested %v\n", RequestedFile)
        } else if VERBOSE {
            fmt.Printf("Generating library %s (package %s)\n",
                RequestedFile.Name+".go", RequestedFile.Pkg)
        }
        if err := RequestedFile.Create(); err != nil {
            fmt.Fprint(os.Stderr, err.String(), "\n")
            os.Exit(1)
        }
    }
}
