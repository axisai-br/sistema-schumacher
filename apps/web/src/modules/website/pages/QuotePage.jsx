import { motion as Motion } from 'framer-motion'
import { Clock, Mail, MapPin, Phone } from 'lucide-react'
import schumacherGraphicEmber from '../../../assets/brand/graphics/schumacher-graphic-ember.svg'
import BookingForm from '../components/BookingForm'
import { contacts, createTelUrl, createWhatsAppUrl } from '../data/contacts'

const contactLinkClass = 'flex min-h-11 items-center gap-3 rounded-sm text-brand-gunship transition-colors hover:text-black focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-gunship focus-visible:ring-offset-2 focus-visible:ring-offset-brand-lynx'
const iconClass = 'flex h-11 w-11 shrink-0 items-center justify-center border border-brand-blue-grey/60 bg-brand-lynx text-brand-gunship'

export default function QuotePage() {
    return (
        <div className="min-h-screen bg-brand-lynx">
            <section className="border-b border-brand-blue-grey/40 bg-brand-gunship">
                <div className="container-max grid items-center gap-8 px-5 py-12 sm:px-8 sm:py-16 lg:grid-cols-4 lg:px-10">
                    <Motion.div
                        initial={{ opacity: 0, y: 20 }}
                        animate={{ opacity: 1, y: 0 }}
                        className="text-center lg:col-span-2 lg:text-left"
                    >
                        <h1 className="mb-4 text-3xl font-bold text-brand-lynx md:text-4xl">
                            Solicite seu Orçamento
                        </h1>
                        <p className="mx-auto max-w-xl text-lg leading-relaxed text-brand-lynx/85 lg:mx-0">
                            Preencha o formulário abaixo e nossa equipe entrará em contato com a melhor proposta para sua viagem!
                        </p>
                    </Motion.div>

                    <Motion.div
                        initial={{ opacity: 0, x: 20 }}
                        animate={{ opacity: 1, x: 0 }}
                        transition={{ delay: 0.1 }}
                        className="flex h-32 items-center justify-center sm:h-40 lg:col-span-2 lg:h-52 lg:justify-end"
                        aria-hidden="true"
                    >
                        <img
                            src={schumacherGraphicEmber}
                            alt=""
                            className="h-full w-full max-w-sm object-contain"
                            decoding="async"
                        />
                    </Motion.div>
                </div>
            </section>

            <section className="px-5 py-16 sm:px-8 sm:py-20 lg:px-10">
                <div className="container-max max-w-6xl">
                    <div className="grid gap-12 lg:grid-cols-12 lg:gap-10">
                        <Motion.div
                            initial={{ opacity: 0, y: 20 }}
                            animate={{ opacity: 1, y: 0 }}
                            transition={{ delay: 0.1 }}
                            className="border border-brand-blue-grey/60 bg-white p-5 sm:p-8 lg:col-span-8"
                        >
                            <BookingForm />
                        </Motion.div>

                        <Motion.aside
                            initial={{ opacity: 0, y: 20 }}
                            animate={{ opacity: 1, y: 0 }}
                            transition={{ delay: 0.2 }}
                            className="divide-y divide-brand-blue-grey/50 border-y border-brand-blue-grey/50 lg:col-span-4 lg:border-y-0 lg:border-l lg:pl-10"
                            aria-label="Informações de contato"
                        >
                            <section className="py-7 lg:first:pt-0">
                                <h2 className="mb-5 text-lg font-bold text-brand-gunship">Contatos Diretos</h2>
                                <div className="space-y-4">
                                    <a href={createTelUrl(contacts.legacyVoice)} className={contactLinkClass}>
                                        <span className={iconClass} aria-hidden="true">
                                            <Phone size={18} />
                                        </span>
                                        <span>
                                            <span className="block text-sm text-brand-gunship/80">Telefone</span>
                                            <span className="font-medium">{contacts.legacyVoice.display}</span>
                                        </span>
                                    </a>
                                    <a
                                        href={createWhatsAppUrl(contacts.commercial)}
                                        target="_blank"
                                        rel="noopener noreferrer"
                                        className={contactLinkClass}
                                    >
                                        <span className={iconClass} aria-hidden="true">
                                            <Phone size={18} />
                                        </span>
                                        <span>
                                            <span className="block text-sm text-brand-gunship/80">WhatsApp</span>
                                            <span className="font-medium">{contacts.commercial.display}</span>
                                        </span>
                                    </a>
                                    <a href="mailto:turismo@schumacher.tur.br" className={contactLinkClass}>
                                        <span className={iconClass} aria-hidden="true">
                                            <Mail size={18} />
                                        </span>
                                        <span className="min-w-0">
                                            <span className="block text-sm text-brand-gunship/80">E-mail</span>
                                            <span className="break-all text-sm font-medium">turismo@schumacher.tur.br</span>
                                        </span>
                                    </a>
                                </div>
                            </section>

                            <section className="py-7">
                                <h2 className="mb-5 text-lg font-bold text-brand-gunship">Nossa Sede</h2>
                                <div className="flex items-start gap-3 text-brand-gunship">
                                    <span className={iconClass} aria-hidden="true">
                                        <MapPin size={18} />
                                    </span>
                                    <p className="text-sm leading-relaxed">
                                        SC-355, KM 35<br />
                                        Sala Comercial CONTAINER<br />
                                        São Sebastião, Fraiburgo/SC<br />
                                        CEP: 89580-000
                                    </p>
                                </div>
                            </section>

                            <section className="py-7 lg:last:pb-0">
                                <h2 className="mb-5 text-lg font-bold text-brand-gunship">Atendimento</h2>
                                <div className="flex items-start gap-3 text-brand-gunship">
                                    <span className={iconClass} aria-hidden="true">
                                        <Clock size={18} />
                                    </span>
                                    <p className="text-sm leading-relaxed">
                                        <strong>Seg - Sex:</strong> 8h às 18h<br />
                                        <strong>Sábado:</strong> 8h às 12h<br />
                                        <strong>WhatsApp:</strong> 24h
                                    </p>
                                </div>
                            </section>
                        </Motion.aside>
                    </div>
                </div>
            </section>
        </div>
    )
}
