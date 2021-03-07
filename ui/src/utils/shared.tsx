/**
 * The function toNumberWithCommas is a shared function that takes a number, and returns it in a comma separated format.
 *
 * @see https://stackoverflow.com/questions/2901102/how-to-print-a-number-with-commas-as-thousands-separators-in-javascript
 *
 * @param {number} value - The number to be formatted
 *
 * @returns {string} formattedValue - The properly formatted value as a string
 *
 * @example
 * toNumberWithCommas(10000) => "10,000"
 */
export function toNumberWithCommas(value: number): string {
	return value.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ",");
}