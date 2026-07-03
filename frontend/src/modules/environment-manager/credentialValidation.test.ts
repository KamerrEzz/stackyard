import {describe, expect, it} from 'vitest'
import {credentialValidationMessage, validateEngineCredentials} from './credentialValidation'

const blank = {username: '', password: '', dbName: ''}

describe('validateEngineCredentials', () => {
    it('allows every field left blank for postgres', () => {
        expect(validateEngineCredentials('postgres', blank)).toBeNull()
    })

    it('allows a full username+password+dbName set for postgres', () => {
        expect(
            validateEngineCredentials('postgres', {username: 'sylvain', password: 's3cret', dbName: 'sylvain_db'}),
        ).toBeNull()
    })

    it('allows a password with no username for mysql (customizing the default root account)', () => {
        expect(validateEngineCredentials('mysql', {...blank, password: 'new-root-pw'})).toBeNull()
    })

    it('rejects a username with no password for postgres', () => {
        expect(validateEngineCredentials('postgres', {...blank, username: 'sylvain'})).toEqual({
            code: 'username-without-password',
        })
    })

    it('rejects a username with no password for mysql', () => {
        expect(validateEngineCredentials('mysql', {...blank, username: 'app_user'})).toEqual({
            code: 'username-without-password',
        })
    })

    it('rejects a username with no password for mongodb', () => {
        expect(validateEngineCredentials('mongodb', {...blank, username: 'app_user'})).toEqual({
            code: 'username-without-password',
        })
    })

    it('treats a whitespace-only username as blank (no error)', () => {
        expect(validateEngineCredentials('postgres', {...blank, username: '   '})).toBeNull()
    })

    it('allows a dbName-only override for mongodb', () => {
        expect(validateEngineCredentials('mongodb', {...blank, dbName: 'custom_db'})).toBeNull()
    })

    it('allows a redis password', () => {
        expect(validateEngineCredentials('redis', {...blank, password: 'requirepass-me'})).toBeNull()
    })

    it('rejects a redis username', () => {
        expect(validateEngineCredentials('redis', {...blank, username: 'someuser'})).toEqual({
            code: 'redis-username-unsupported',
        })
    })

    it('rejects a redis dbName', () => {
        expect(validateEngineCredentials('redis', {...blank, dbName: '0'})).toEqual({
            code: 'redis-dbname-unsupported',
        })
    })

    it('rejects a redis username even when accompanied by a valid password', () => {
        expect(validateEngineCredentials('redis', {username: 'someuser', password: 'pw', dbName: ''})).toEqual({
            code: 'redis-username-unsupported',
        })
    })
})

describe('credentialValidationMessage', () => {
    it('names the engine in the username-without-password message', () => {
        expect(credentialValidationMessage({code: 'username-without-password'}, 'PostgreSQL')).toContain('PostgreSQL')
    })

    it('describes the redis username restriction', () => {
        expect(credentialValidationMessage({code: 'redis-username-unsupported'}, 'Redis')).toMatch(/username/i)
    })

    it('describes the redis dbName restriction', () => {
        expect(credentialValidationMessage({code: 'redis-dbname-unsupported'}, 'Redis')).toMatch(/database name/i)
    })
})
