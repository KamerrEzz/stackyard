export type Engine = 'postgres' | 'mysql' | 'mongodb' | 'redis'

export interface EngineCredentials {
    username: string
    password: string
    dbName: string
}

export type CredentialValidationError =
    | {code: 'username-without-password'}
    | {code: 'redis-username-unsupported'}
    | {code: 'redis-dbname-unsupported'}

/**
 * Validates one engine's custom-credential inputs on the Create-profile form
 * (tasks.md 10.1) against that engine's real container/auth model, mirroring
 * the same rules app.go's CreateProfile/redisCredentialFieldsError enforce
 * server-side — this catches the same mistakes before a round trip, it does
 * not replace the backend check.
 *
 * Redis has no username or upfront database-name concept (redis.go): both
 * are rejected outright, matching CreateProfile's hard error rather than
 * silently dropping the value the user typed. Its Password IS meaningful
 * (redis.go's `--requirepass`) and is never rejected here.
 *
 * For Postgres/MySQL/MongoDB, a Password with no Username is allowed — it
 * customizes that engine's already-existing default account (e.g.
 * Postgres's "postgres" superuser, MySQL's "root"), a legitimate use case.
 * A Username with no Password is rejected: CreateProfile would silently
 * backfill defaultsForEngine's built-in default password onto the
 * caller's custom username, pairing a user-chosen name with a
 * publicly-documented default password — a confusing, easy-to-miss footgun
 * rather than the customization the user likely intended.
 */
export function validateEngineCredentials(engine: Engine, input: EngineCredentials): CredentialValidationError | null {
    const username = input.username.trim()
    const password = input.password.trim()
    const dbName = input.dbName.trim()

    if (engine === 'redis') {
        if (username !== '') {
            return {code: 'redis-username-unsupported'}
        }
        if (dbName !== '') {
            return {code: 'redis-dbname-unsupported'}
        }
        return null
    }

    if (username !== '' && password === '') {
        return {code: 'username-without-password'}
    }

    return null
}

export function credentialValidationMessage(error: CredentialValidationError, engineLabel: string): string {
    switch (error.code) {
        case 'username-without-password':
            return `Enter a password for this custom username, or clear the username to use ${engineLabel}'s default.`
        case 'redis-username-unsupported':
            return 'Redis has no username concept — leave this blank.'
        case 'redis-dbname-unsupported':
            return 'Redis has no upfront database name — leave this blank.'
    }
}
