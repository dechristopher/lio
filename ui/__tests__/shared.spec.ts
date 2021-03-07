import {toNumberWithCommas} from "../src/utils/shared";

test("toNumberWithCommas", () => {
	expect(toNumberWithCommas(1000)).toBe("1,000");
})