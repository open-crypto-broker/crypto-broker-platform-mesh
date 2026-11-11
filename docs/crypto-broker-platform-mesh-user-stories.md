
# User Stories for the Integration of Crypto Broker into Platform Mesh

## Problem statement

In our last meeting the question came up regarding the implementation and usage of different cryptographic profiles within the Crypto Broker on Platform Mesh.
Different follow up questions arose, e.g. where the profiles are stored and how they are selected by a user or an application.
The idea came up to document a user story for the deployment of the Crypto Broker Operator, Server and profiles within Platform Mesh.

A clarification ahead: there is not just a single user story.
Different actors are involved in the process of managing and using cryptographic profiles with the Crypto Broker Server, including crypto policy owners, application developers, the Platform Mesh administrator, the Crypto Broker team, and auditors.
Each actor interacts with the cryptographic profiles and the Crypto Broker Operator, Server and profiles in different ways.
It is therefore clear, that not one single user story can capture all interactions and requirements related to cryptographic profiles within Platform Mesh.
The purpose of this document is to outline the various user stories related to the interaction with cryptographic profiles within the Crypto Broker Operator and Server on Platform Mesh, highlighting the roles and responsibilities of different actors and the scenarios in which they engage with these profiles.

## Scope of this document

This document is about **who is involved and who is responsible for what**.

## The principle we need to agree on first

> **The consumer selects a profile by name. The crypto policy owner defines what that name means.**

This separation of duties is the reason the Crypto Broker exists in this architecture.
If a consuming team could submit profile *content* instead of a profile *name*, then a name such as `FIPS-140-3-256bit` would carry no guarantee at all, because the consumer would have authored its own definition of it.

## Deployment Actors

| Actor | Owns | Decides | Depends on | Fails if |
| --- | --- | --- | --- | --- |
| **Crypto Policy Owner** | The content of the profiles and the policy behind them | Which algorithms are approved, when a profile is deprecated and what replaces it | A publishing path that no one else can write to | A profile is changed or published by someone outside the policy group |
| **Application Developer (Consumer Team)** | The application and its choice of profile *name* | Which published profile fits the application's compliance requirement | Being able to see which profiles exist and what they guarantee | The specified profile name does not exist or is incorrect |
| **Platform Mesh Administrator** | Platform Mesh instance, onboarding of providers and consumers | How a provider is onboarded and which trust boundaries exist | Providers conforming to the `APIExport`/`APIBinding` contract | A consumer can bypass the contract and provision profile configuration itself |
| **Auditor** | The evidence requirement | Whether the evidence chain is sufficient | Read access to approval, control-plane and runtime evidence | They cannot determine which exact profile content a workload is running |

## Providing Actors

| Actor | Owns | Decides | Depends on | Fails if |
| --- | --- | --- | --- | --- |
| **Crypto Broker Team** | The Crypto Broker Operator which injects the Crypto Broker Server in a running environment | When a new operator version is rolled out | The Crypto Broker Server for releases, the crypto policy owner for content of the profiles | Publishing a profile requires a code or API schema change |

## User Stories

### User Story 1: Crypto Policy Owner

**As a** crypto policy owner
**I want to** define, publish and deprecate cryptographic profiles
**so that** applications and users within Platform Mesh can adhere to the organization's cryptographic policies.

Acceptance criteria:

- Only members of this group can create or modify a profile; no consumer identity can.
- Every change is attributable to a person and point in time.

### User Story 2: Application Developer (Consumer Team)

**As an** application developer
**I want to** select a cryptographic profile by name when I request a Crypto Broker for my application
**so that** my application running in Platform Mesh is compliant according to the defined cryptographic policies.

Acceptance criteria:

- Can see which profiles are available, and what each one guarantees, before deployment.
- Can only supply a profile name; cannot supply profile content.
- An unknown or unavailable profile name fails visibly with an actionable error message.

### User Story 3: Platform Mesh Administrator

**As the** Platform Mesh administrator
**I want to** operate the Platform Mesh instance and onboard providers and consumers through the `APIExport`/`APIBinding` contract
**so that** a provider can offer the Crypto Broker and a consumer can bind it, without either side gaining access to the other's cluster.

Acceptance criteria:

- Onboarding a new consumer requires no change to the provider's implementation.
- A consumer can only use the APIs its bindings grant; no broader access is implied.
- A consumer cannot bypass the contract to provision its own profile configuration.
- Also involved into publishing the profile catalog? Clarification needed.

### User Story 4: Auditor

**As an** auditor
**I want to** determine for any running workload which exact profile content it is using and where that content came from
**so that** I can evidence that the deployed applications in Platform Mesh adhere to the defined cryptographic policies.

Acceptance criteria, which correspond to three distinct layers of evidence:

- **Approval:** I can trace the profile content back to a reviewed and approved change.
- **Control plane:** I can see who published it to the running environment, and when.
- **Runtime:** I can identify the exact profile content a given workload is running, and tell whether it still matches the currently published profile of that name.

### User Story 5: Crypto Broker Team

**As the** Crypto Broker team
**we want to** run the Crypto Broker Server offering (through the Crypto Broker Operator) and combine it with the independently published profile catalog into a pod
**so that** profiles reach the requesting applications correctly.

Acceptance criteria:

- Publishing a new profile (from the crypto policy owners) requires no change to the `CryptoBroker` API schema.
- Existing `CryptoBroker` objects and already-running workloads are unaffected by a catalog update.
- The catalog has exactly one source of truth.

## Workflow

How could a typical interaction workflow look like for the different actors involved in managing and using cryptographic profiles within Platform Mesh?

The steps below are ordered as a lifecycle.
That ordering matters: nothing can be selected before it has been published, and nothing can be published before the offering has been onboarded.

| # | Step | Driven by | Result |
| --- | --- | --- | --- |
| 1 | **Define** | Crypto Policy Owner | A profile is specified according to organizational policy and reviewed |
| 2 | **Publish** | Crypto Policy Owner | The profile becomes part of the catalog of the running offering |
| 3 | **Offer** | Platform Mesh Administrator | The Crypto Broker API is exported and discoverable in Platform Mesh |
| 4 | **Bind** | Platform Mesh Administrator | The consumer workspace binds the exported API |
| 5 | **Select** | Application Developer | The consumer requests a Crypto Broker and names a profile |
| 6 | **Provision** | Crypto Broker Operator | The named profile is resolved and delivered to the workload |
| 7 | **Evidence** | Auditor | The delivered profile and its origin can be verified |
| 8 | **Deprecate** | Crypto Policy Owner | A successor profile and a sunset date are published |
| 9 | **Migrate** | Application Developer | The consumer switches to the successor profile on a planned schedule |

## Open Questions

The questions below follow the order of the user stories.
They are kept at the level of ownership and responsibility.

### Crypto Policy Owner

The crypto policy owner is responsible for defining and updating cryptographic profiles according to organizational policies.
There should be a way to provide a profile catalog that the application developers can browse and select from during the deployment of their applications.
Open questions:

- Who exactly forms this group, and how is write access to the catalog restricted to it?
- Where is the profile catalog stored, and how is access to it controlled?
- Is there a storage mechanism for the profile catalog available in Platform Mesh, and how is it managed?

### Application Developer (Consumer Team)

The application developer is responsible for selecting the appropriate cryptographic profile for their application during deployment.
Open questions:

- How can application developers discover available cryptographic profiles?
- What level of access do application developers have to the profile catalog (can they see the content of the profiles)?
- How are application developers informed about updates to cryptographic profiles?
- How can application developers request changes to cryptographic profiles?

### Platform Mesh Administrator

The platform mesh administrator is responsible for operating the Platform Mesh instance and for onboarding providers and consumers.
Open questions:

- How is the Crypto Broker operator integrated into Platform Mesh?
- Who is responsible for deploying and monitoring the deployed Crypto Broker operator?
- How can it be ensured that application developers cannot bypass the Crypto Broker operator to deploy their own cryptographic profiles?

### Auditor

The auditor is responsible for reviewing the deployment and usage of cryptographic profiles to ensure compliance with the defined policies.
Open questions:

- What level of access does the auditor need to have for Platform Mesh to see the deployment and usage of cryptographic profiles?
- How can the auditor prove that the content running in a workload is the approved content and has not been altered?
- How are audit findings reported and addressed?

## Cross-cutting open questions

These questions are not owned by a single actor, and one of them may change the actor list itself.

- **One catalog or many?** Is there a single profile catalog for the whole mesh, or may a consuming organization have its own crypto policy owner with organization-scoped profiles? The answer determines whether "Crypto Policy Owner" is a provider-side or a consumer-side role — and therefore whether the list above is complete.
- **Accountability.** Who is accountable when a workload turns out to be non-compliant: the policy owner, the platform mesh administrator, or the application developer?
- **Operations.** Who is paged when profile delivery fails in production?
- **Onboarding.** Is onboarding of a new consumer self-service, or does the crypto policy/platform mesh administrator approve it?
- **Exception handling.** Through which channel does a consumer request a profile that does not exist yet, and who decides?

## Next step

Once the actor list, the separation-of-duties principle and the ownership assignments above are agreed, the technical discussion can start:
where the profile catalog lives, how it is published, and how consumers discover it.
