import type { Metadata } from "next";
import { NextIntlClientProvider } from "next-intl";
import { getLocale, getMessages } from "next-intl/server";
import { MockProvider } from "@/mocks/MockProvider";
import "./globals.css";

export const metadata: Metadata = {
  title: "Mini Card Network",
  description: "POS simulator and operations console",
};

export default async function RootLayout({ children }: LayoutProps<"/">) {
  const locale = await getLocale();
  const messages = await getMessages();
  return (
    <html lang={locale} className="h-full antialiased">
      <body className="min-h-full bg-canvas font-sans text-ink">
        <NextIntlClientProvider locale={locale} messages={messages}>
          <MockProvider>{children}</MockProvider>
        </NextIntlClientProvider>
      </body>
    </html>
  );
}
