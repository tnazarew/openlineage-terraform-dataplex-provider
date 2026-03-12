# Bring Your Own OpenLineage (BYOOL)

## BYOL

There is an idea called Bring Your Own Lineage (BYOL).
The purpose of it is to enable the users to declare lineage entities and supply them to their Catalogs.
That fills the gap left by the components which don't have runtime lineage support.

Databricks already has something like that [here](https://docs.databricks.com/aws/en/data-governance/unity-catalog/external-lineage).

In case of OpenLineage it would greatly lower the barrier of entry, users would not be limited to components already producing OpenLineage.
That works very well combined with OpenLineages portability, data defined once in OpenLineage format would be usable across different catalogs.

From implementation perspective, it would be more or less what we already have (client API and transports) but with validation layer to restrict what can be added.

## Terraform BYOOL Provider

I would like to create the `Bring Your Own OpenLineage` with Terraform. Terraform is a popular tool for
controlling the state of the environment. Each terraform resource is a managed entity, which
means that terraform keeps track of its state and can create, update or delete it.
Terraform has an API for creating custom providers and resources so lineage entities could be defined as terraform resources.

### Terraform resource lifecycle overview

For each resource Terraform has few states I call them:

- `config` is the desired state of the resource defined in the terraform configuration file
- `previous` is the state of the resource stored in the terraform state file
- `current` is the state of the resource in the environment

When Terraform executes `apply` the provider does few things

1. create a plan
    1. drift detection
        1. fetch the `current` state of the resource in the environment
        2. compare it with the `previous` state stored in the state file
        3. in case of discrepancy, it can error or use some other defined strategy (overwrite, ignore, etc.)
    2. change detection
        1. compare the `config` with the `previous` state stored in the state file
        2. set the plan to create, update or delete the resource based on the differences
2. execute the plan
    1. make changes in the resource
    2. store the new state in the state file

### OpenLineage Terraform Provider

BYOOL provider would implement 3 types of resources for static lineage

1. dataset events
2. job events
3. run-ish events (if the consumer doesn't support static lineage events, we can put job event in the run wrapper without any runtime facets)

Mechanism for making changes would be to just emit OL events to consumer. There are some complications, however.

The provider cannot be fully generic, while the `config` is just OL structure, the `current` is always consumer specific as it has to be a representation of the consumer state. The `previous` needs to be a combination of both.
That means we couldn't have a single provider for all consumers, we would need to have one per consumer.

It means in worst case we would just have multiple separate providers, sharing the same configuration structure (which is something).
The best case scenario here would be to have a common module containing bulk (hopefully all) of the configuration and event creation logic and separate providers using common functionalities.

### Requirements
The Go client implemented in #4358 would is required for the provider.

### Example
The [OpenLineage Terraform Dataplex provider](https://github.com/tnazarew/openlineage-terraform-dataplex-provider) is example of the separate provider. 


