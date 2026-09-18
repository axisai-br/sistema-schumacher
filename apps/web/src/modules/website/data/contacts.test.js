import test from 'node:test'
import assert from 'node:assert/strict'
import {
    contacts,
    createTelUrl,
    createWhatsAppUrl,
    resolveBookingContact,
} from './contacts.js'

test('defines the commercial and travel AI contacts', () => {
    assert.equal(contacts.commercial.number, '5549988502101')
    assert.equal(contacts.commercial.display, '+55 49 98850-2101')
    assert.equal(contacts.secondaryWhatsApp.number, '5549999862222')
    assert.equal(contacts.secondaryWhatsApp.display, '+55 49 99986-2222')
    assert.equal(contacts.travelAi.number, '5549991600330')
})

test('creates WhatsApp URLs and encodes the message', () => {
    assert.equal(
        createWhatsAppUrl(contacts.commercial, 'Olá! Cotação & viagem'),
        'https://wa.me/5549988502101?text=Ol%C3%A1!%20Cota%C3%A7%C3%A3o%20%26%20viagem',
    )
    assert.equal(
        createWhatsAppUrl(contacts.travelAi),
        'https://wa.me/5549991600330',
    )
    assert.equal(
        createWhatsAppUrl(contacts.secondaryWhatsApp),
        'https://wa.me/5549999862222',
    )
})

test('creates telephone URLs for normalized contacts', () => {
    assert.equal(createTelUrl(contacts.commercial), 'tel:+5549988502101')
    assert.equal(createTelUrl(contacts.legacyVoice), 'tel:+554932466666')
})

test('routes booking destinations to the expected contact', () => {
    for (const destination of ['maranhao', 'sc-balneario', 'sc-beto-carrero', 'sc-combo']) {
        assert.equal(resolveBookingContact(destination), contacts.travelAi)
    }

    for (const destination of ['fretamento', 'outro']) {
        assert.equal(resolveBookingContact(destination), contacts.commercial)
    }
})
