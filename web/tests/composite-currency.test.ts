import { describe, it } from 'vitest';
import { RuleTester } from 'eslint';
import tseslint from 'typescript-eslint';
// @ts-expect-error — JS plugin without type declarations.
import fbPlugin from '../eslint-rules/composite-currency.js';

const tsParser = tseslint.parser;

const rule = fbPlugin.rules['composite-currency'];

const ruleTester = new RuleTester({
  languageOptions: { parser: tsParser, parserOptions: { ecmaVersion: 2022, sourceType: 'module' } },
});

describe('fb/composite-currency', () => {
  it('passes valid and rejects invalid monetary shapes', () => {
    ruleTester.run('composite-currency', rule, {
      valid: [
        // camelCase cents typed as string with a currency sibling.
        { code: `interface I { totalCents: string; currencyCode: string }` },
        // snake_case wire shape (mirrors Go JSON tags).
        { code: `interface I { total_cents: string; currency_code: string }` },
        // Non-monetary number is fine.
        { code: `interface I { count: number }` },
        // GPS coords are exempt floats.
        { code: `interface I { lat: number; lng: number }` },
        // Class form with sibling currency.
        { code: `class C { budgetCents = '0'; currencyCode = 'USD'; }` },
      ],
      invalid: [
        // Float money property.
        {
          code: `interface I { totalAmount: number }`,
          errors: [{ messageId: 'floatMoney' }],
        },
        // camelCase cents without a currency sibling.
        {
          code: `interface I { totalCents: string }`,
          errors: [{ messageId: 'missingCurrency' }],
        },
        // snake_case cents without a currency sibling.
        {
          code: `interface I { total_cents: string }`,
          errors: [{ messageId: 'missingCurrency' }],
        },
      ],
    });
  });
});
