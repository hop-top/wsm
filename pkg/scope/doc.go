// Package scope defines scope types, assignment models, and query operations
// for workspace and space access control within wsm.
//
// Scopes constrain what profiles and squads can access within a workspace:
// file patterns, operations, tool access, secrets, and network resources.
//
// Assignment links profiles/squads to workspaces with specific scopes.
// Query functions provide intersection, union, and effective scope resolution.
// Session context enforces workspace-scoped vs space-scoped session distinction.
package scope
