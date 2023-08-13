# os-backend

## Project Intro

### Deps

* Web framework: *gin*
* ORM: *gorm*
* Database for testing: *mysql*
* Permission Library: *casbin*

## Structure

The main entry for the project is `cmd/apiserver.go`, which contains routers for all functions.

The file contains both authorized and non-authorized routers, which are written in different structure, for example:

```golang
	v1 := r.Group("/v1")
	// --> no auth required
	{
		
	}
    // --> auth required
    {
        authorizedGroup := v1.Group("/", middleware.AuthRequired)
    }
```

The configuration file is yaml format, the fields can be found in `config.yml.example` file.


The logic used in code is saved in internal directory, and here is the structure.

```shell
internal
├── api
│   ├── application
│   ├── event
│   ├── guild
│   ├── project
│   ├── treasury
│   └── user
├── common
├── config
├── middleware
├── model
├── sdk
├── service
└── storage
```

Logics for routers are mostly saved in `api` folder, and each group is saved in sub folder.
The data models are saved in `model` folder.


