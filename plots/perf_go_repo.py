#!/usr/bin/env python

import pandas as pd
import matplotlib.pyplot as plt
import numpy as np
import argparse

def main():
    parser = argparse.ArgumentParser(description='Generate performance histograms from diff statistics CSV file')
    parser.add_argument('stats', help='Path to the CSV file containing statistics data')
    args = parser.parse_args()

    stats = pd.read_csv(args.stats)

    for variant in ["regular", "anchoring", "optimal"]:
        statsv = stats[stats["variant"] == variant]

        hist, bins = np.histogram(statsv['duration_ns']/1000, bins=100)        
        logbins = np.logspace(np.log10(bins[0]),np.log10(bins[-1]),len(bins))

        # Create figure and axes objects explicitly
        fig, ax = plt.subplots(figsize=(8, 6))
        
        # Use axes methods instead of global plt
        ax.hist(statsv['duration_ns']/1000, bins=logbins)
        ax.set_xlabel('µs')
        ax.set_xscale('log')
        ax.set_ylabel('Frequency')
        ax.set_xlim(1, 10**6)
        ax.set_title('textdiff.Unified(...) for Go repository')
        
        # Save using the figure object
        fig.savefig(f'perf_go_repo_{variant}.png')
        
        # Close the figure to free memory
        plt.close(fig)


if __name__ == '__main__':
    main()

