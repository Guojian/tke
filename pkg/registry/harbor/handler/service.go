package handler

import (
	"context"

	harbor "tkestack.io/tke/pkg/registry/harbor/client"
	"tkestack.io/tke/pkg/util/log"

	"github.com/antihax/optional"
)

func CreateProject(ctx context.Context, client *harbor.APIClient, projectName string, public bool) (err error) {

	projectReq := harbor.HarborProjectReq{
		ProjectName: projectName,
		Public:      public,
	}

	_, err = client.ProjectApi.CreateProject(ctx, projectReq, nil)

	if err != nil {
		log.Error("Failed to create harbor project", log.Err(err))
		return err
	}
	return nil

}

func DeleteProject(ctx context.Context, client *harbor.APIClient, projectName string) (err error) {

	opts := harbor.ProjectApiListProjectsOpts{
		Name: optional.NewString(projectName),
	}

	projects, _, err := client.ProjectApi.ListProjects(ctx, &opts)
	if err != nil {
		log.Error("Failed to list harbor project", log.Err(err))
		return err
	}

	if len(projects) == 1 {
		projectID := projects[0].ProjectId

		_, err = client.ProjectApi.DeleteProject(ctx, int64(projectID), nil)
		if err != nil {
			log.Error("Failed to delete harbor project", log.Err(err))
			return err
		}
	}
	return nil

}

func DeleteRepo(ctx context.Context, client *harbor.APIClient, projectName, repoName string) (err error) {

	_, err = client.RepositoryApi.DeleteRepository(ctx, projectName, repoName, nil)
	if err != nil {
		log.Error("Failed to delete harbor repo", log.Err(err))
		return err
	}

	return nil

}
