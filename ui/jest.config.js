module.exports = {
  verbose: true,
  preset: 'ts-jest',
  testEnvironment: 'node',
  collectCoverage: true,
  collectCoverageFrom: ['src/utils/**/*.{ts,tsx}'],
  roots: [
      "<rootDir>/__tests__",
      "<rootDir>/src",
  ]
};