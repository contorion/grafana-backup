// Backup tool for Grafana.
// Copyright (C) 2016-2017  Alexander I.Grafov <siberian@laika.name>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.
//
// ॐ तारे तुत्तारे तुरे स्व

package main

import (
  "context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/gosimple/slug"
	"github.com/grafana-tools/sdk"
)

func doBackup(opts ...option) {
	var (
		cmd = initCommand(opts...)
	)
	if cmd.applyHierarchically {
		backupDashboards(cmd)
		return
	}
	if cmd.applyForBoards {
		backupDashboards(cmd)
	}
	if cmd.applyForDs {
		backupDatasources(cmd, nil)
	}
	if cmd.applyForUsers {
		backupUsers(cmd)
	}

}

func backupDashboards(cmd *command) {
	var (
		boardLinks  []sdk.FoundBoard
		rawBoard    []byte
		meta        sdk.BoardProperties
		board       sdk.Board
		datasources = make(map[string]bool)
		err         error
	)
  ctx := context.Background()
	if boardLinks, err = cmd.grafana.SearchDashboards(ctx, cmd.boardTitle, cmd.starred, cmd.tags...); err != nil {
		fmt.Fprintf(os.Stderr, fmt.Sprintf("%s\n", err))
		os.Exit(1)
	}
	if cmd.verbose {
		fmt.Printf("Found %d dashboards that matched the conditions.\n", len(boardLinks))
	}

	// TODO: If this directory already exists prompt to overwrite (unless --force)
	VerifyOrCreateDir(*flagDir)

	for _, link := range boardLinks {
		select {
		case <-cancel:
			exitBySignal()
		default:
			if rawBoard, meta, err = cmd.grafana.GetRawDashboardBySlug(ctx,link.URI); err != nil {
				fmt.Fprintf(os.Stderr, fmt.Sprintf("%s for %s\n", err, link.URI))
				continue
			}
			if cmd.applyHierarchically {
				if err = json.Unmarshal(rawBoard, &board); err != nil {
					fmt.Fprintf(os.Stderr, fmt.Sprintf("error %s parsing %s\n", err, meta.Slug))
				} else {
					extractDatasources(cmd, datasources, board)
				}
			}
			var fname = fmt.Sprintf(path.Join(*flagDir, "%s.db.json"), meta.Slug)
			if err = ioutil.WriteFile(fname, rawBoard, os.FileMode(int(0666))); err != nil {
				fmt.Fprintf(os.Stderr, fmt.Sprintf("%s for %s\n", err, meta.Slug))
				continue
			}
			if cmd.verbose {
				fmt.Printf("%s writen into %s.\n", meta.Slug, fname)
			}
		}
	}
	if cmd.applyHierarchically {
		backupDatasources(cmd, datasources)
	}
}

func backupUsers(cmd *command) {
	var (
		allUsers []sdk.User
		rawUser  []byte
		err      error
	)
  ctx := context.Background()
	if allUsers, err = cmd.grafana.GetAllUsers(ctx); err != nil {
		fmt.Fprintf(os.Stderr, fmt.Sprintf("%s\n", err))
		return
	}

	VerifyOrCreateDir(*flagDir)

	for _, user := range allUsers {
		select {
		case <-cancel:
			exitBySignal()
		default:
			rawUser, _ = json.Marshal(user)
			var fname = fmt.Sprintf(path.Join(*flagDir, "%s.user.%d.json"), slug.Make(user.Login), user.OrgID)
			if err = ioutil.WriteFile(fname, rawUser, os.FileMode(int(0666))); err != nil {
				fmt.Fprintf(os.Stderr, fmt.Sprintf("error %s on writing %s\n", err, fname))
				continue
			}
			if cmd.verbose {
				fmt.Printf("%s written into %s\n", user.Name, fname)
			}
		}
	}
}

func backupDatasources(cmd *command, datasources map[string]bool) {
	var (
		allDatasources []sdk.Datasource
		rawDs          []byte
		err            error
	)

  ctx := context.Background()
	if allDatasources, err = cmd.grafana.GetAllDatasources(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return
	}

	if cmd.verbose {
		fmt.Printf("Found %d datasources.\n", len(allDatasources))
	}

	VerifyOrCreateDir(*flagDir)

	for _, ds := range allDatasources {
		select {
		case <-cancel:
			exitBySignal()
		default:
			if datasources != nil {
				if _, ok := datasources[ds.Name]; !ok {
					continue
				}
			}

			if rawDs, err = json.Marshal(ds); err != nil {
				fmt.Fprintf(os.Stderr, "datasource marshal error %s\n", err)
				continue
			}

			var fname = fmt.Sprintf(path.Join(*flagDir, "%s.ds.%d.json"), slug.Make(ds.Name), ds.OrgID)

			if err = ioutil.WriteFile(fname, rawDs, os.FileMode(int(0666))); err != nil {
				fmt.Fprintf(os.Stderr, fmt.Sprintf("%s for %s\n", err, ds.Name))
				continue
			}

			if cmd.verbose {
				fmt.Printf("%s written into %s", ds.Name, fname)
			}
		}
	}
}

func extractDatasources(cmd *command, datasources map[string]bool, board sdk.Board) {
	for _, row := range board.Rows {
		for _, panel := range row.Panels {
			if panel.Datasource != nil {
				datasources[*panel.Datasource] = true
				if cmd.verbose {
					fmt.Printf("Found Datasource [%s] in dashboard [%s]: Adding to backup list.\n", slug.Make(*panel.Datasource), board.Title)
				}
			}
		}
	}
}

// Checks to see if a directory exists. If not creates it along with any parent directories. Returns an error if the
// file exists but is not a directory or if it is unable to create the directory.
func VerifyOrCreateDir(directory string) (error) {
	stat, err := os.Stat(directory)
	if err == nil {
		if !stat.IsDir() {
			return errors.New("Specified path is not a directory!")
		}
		return nil
	}
	if os.IsNotExist(err) {
		err = os.MkdirAll(directory, 0755)
		return err
	}


	return err
}
