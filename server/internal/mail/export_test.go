package mail

func SetImplicitTLS(mailer *SMTPMailer, isImplicitTLS bool) { mailer.isImplicitTLS = isImplicitTLS }

func SetLoopbackCheck(mailer *SMTPMailer, checkLoopbackHost func(string) bool) {
	mailer.checkLoopbackHost = checkLoopbackHost
}
