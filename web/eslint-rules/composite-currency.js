/**
 * eslint-plugin-fb — composite-currency rule.
 *
 * Enforces the Composite Currency Pattern on the frontend (TECH_STACK §Constraints,
 * CLAUDE.md). Two checks on object-type/interface/class members:
 *
 *  1. A property whose name matches a monetary pattern
 *     (cost|price|amount|total|budget|fee|payment|invoice|balance|revenue|expense)
 *     and is typed `number` is flagged UNLESS its name ends in cents. Money must
 *     be integer cents (typed as string|bigint to avoid 2^53 overflow), never a
 *     float `number`.
 *  2. Any property ending in cents must have a SIBLING currency-code property in
 *     the same type/interface/class — money never travels without its currency.
 *
 * Both naming conventions are recognized: TS-native camelCase (`totalCents` +
 * `currencyCode`) and the snake_case wire shapes mirroring Go JSON tags
 * (`total_cents` + `currency_code`).
 *
 * GPS coordinate columns (gps_lat/gps_lng/lat/lng) are exempt — they are the only
 * legitimate floating-point numerics (mirrors the SQL migration linter rule 1).
 */

const MONETARY_RE = /(cost|price|amount|total|budget|fee|payment|invoice|balance|revenue|expense)/i;
const COORD_RE = /^(gps_)?(lat|lng|latitude|longitude)$/i;
const CENTS_RE = /_?cents$/i;
const CURRENCY_RE = /currency_?code$/i;

/** Extracts a member's identifier name, or null for computed/non-identifier keys. */
function memberName(node) {
  const key = node.key;
  if (!key) return null;
  if (key.type === 'Identifier') return key.name;
  if (key.type === 'Literal' && typeof key.value === 'string') return key.value;
  return null;
}

function isNumberAnnotation(node) {
  const ann = node.typeAnnotation && node.typeAnnotation.typeAnnotation;
  return Boolean(ann) && ann.type === 'TSNumberKeyword';
}

/** @type {import('eslint').Rule.RuleModule} */
const compositeCurrency = {
  meta: {
    type: 'problem',
    docs: {
      description: 'Enforce the Composite Currency Pattern (integer *Cents + *CurrencyCode).',
    },
    schema: [],
    messages: {
      floatMoney:
        "Monetary property '{{name}}' must be integer cents (a *cents field typed string|bigint), not a float 'number'.",
      missingCurrency:
        "Property '{{name}}' is monetary cents but has no sibling currency-code property in the same type.",
    },
  },
  create(context) {
    /** Collects member nodes for a container, then runs both checks. */
    function checkMembers(members) {
      const names = [];
      for (const m of members) {
        const n = memberName(m);
        if (n) names.push(n);
      }
      const hasCurrencySibling = names.some((n) => CURRENCY_RE.test(n));

      for (const m of members) {
        const name = memberName(m);
        if (!name) continue;

        // Check 1: float money.
        if (
          isNumberAnnotation(m) &&
          MONETARY_RE.test(name) &&
          !CENTS_RE.test(name) &&
          !COORD_RE.test(name)
        ) {
          context.report({ node: m, messageId: 'floatMoney', data: { name } });
        }

        // Check 2: cents without a currency-code sibling.
        if (CENTS_RE.test(name) && !hasCurrencySibling) {
          context.report({ node: m, messageId: 'missingCurrency', data: { name } });
        }
      }
    }

    return {
      TSInterfaceBody(node) {
        checkMembers(node.body);
      },
      TSTypeLiteral(node) {
        checkMembers(node.members);
      },
      ClassBody(node) {
        checkMembers(node.body.filter((b) => b.type === 'PropertyDefinition'));
      },
    };
  },
};

export default {
  rules: {
    'composite-currency': compositeCurrency,
  },
};
