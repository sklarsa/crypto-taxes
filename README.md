# Crypto Tax Calculator

**DISCLAIMER: This material has been prepared for informational purposes only, and is not intended to provide, and should not be relied on for, tax, legal or accounting advice. You should consult your own tax, legal and accounting advisors before engaging in any transaction.**

## Description & Motivation

Parses coinbase csvs to calculate P&L on crypto transactions using the FIFO (First-In-First-Out) method.

Because coinbase's csv export is not compatible with Turbotax.  See:

- [Coinbase CSV tax doesn’t work with Turbo Tax?](https://www.reddit.com/r/CoinBase/comments/kwbw3w/coinbase_csv_tax_doesnt_work_with_turbo_tax/)
- [Coinbase csv file not compatible on turbotax? No Headers Found In This File error?](https://ttlc.intuit.com/community/taxes/discussion/coinbase-csv-file-not-compatible-on-turbotax-no-headers-found-in-this-file-error/00/1820285)

## How to run

1. Download the release for your CPU architecture & operating system type from the [this project's releases page](https://github.com/sklarsa/crypto-taxes/releases)
2. Optionally rename the file (the rest of the examples use `crypto-taxes` as the name of the executable)
3. (Mac OSX Only) Find the executable in the Finder, right-click it and select "Open" to bypass OSX's overly-cautious security features
4. In the terminal, navigate to the directory of the executable and run the following command:

    ```bash
    # Change file permissions to allow execution
    $ chmod 740 crypto-taxes

    # Run the file on a downloaded csv from Coinbase.  The -csv flag outputs a valid csv to stdout.
    ./crypto-taxes -csv your-coinbase-file.csv
    ```
