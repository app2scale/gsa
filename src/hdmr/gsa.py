# %%
import pandas as pd
import numpy as np
import matplotlib.pyplot as plt
import logging
import math
import matplotlib as mpl
from pylab import cm 

avenirfont = {'fontname': 'Avenir', 'size': 18}
units = {'perf': 'paket/sn', 'cost': 'USD/saat'}
names = {'perf': 'performans', 'cost': 'maliyet'}
plt.rcParams['text.usetex'] = False
mpl.rcParams['font.family'] = 'Avenir'
plt.rcParams['font.size'] = 18
plt.rcParams['axes.linewidth'] = 2  

csv_file = 'data.log'

df = pd.read_csv(csv_file, delim_whitespace=True)
varnames = ['re','cpu','heap']
var_idx_map = [0,1,2]
variables = dict()
for var in varnames:
    variables[var] = sorted(df[var].unique())
gridsize = [len(x) for x in variables.values()]

n = len(variables.keys()) # dimension
N = len(df) # number of records
state_visit_counts = dict()

# s = {'re': 1, 'heap': 100, 'cpu': 100} --> return 0
# s = {'re': 1, 'heap': 100, 'cpu': 200} --> return 1
def state_to_idx(s):
    idx = 0
    for i in range(n):
        varname = varnames[i]
        idx += variables[varname].index(s[varname]) * np.prod([len(variables[varnames[j]]) for j in range(i+1,n) ])
    return int(idx)

# Inverse of state_to_idx
def idx_to_state(idx):
    s = dict()
    for i in range(n):
        varname = varnames[i]
        block = int(np.prod([len(variables[varnames[j]]) for j in range(i+1,n) ]))
        s[varname] = variables[varname][idx // block]
        idx = idx - block * (idx // block)
    return s

def filter_data(state):
    filter_idx = True
    for var in varnames:
        filter_idx = (filter_idx) & (df[var] == state[var])
    return filter_idx

def filter_data_state_idx(state_idx):
    return  filter_data(idx_to_state(state_idx))  


def neighborlist(state):
    yield state
    for var in variables:
        state_value = state[var]
        loc = variables[var].index(state_value) 
        if loc - 1 >= 0:
            s = state.copy()
            s[var] = variables[var][loc-1]
            yield s
        if loc + 1 < len(variables[var]):
            s = state.copy()
            s[var] = variables[var][loc+1]
            yield s

 

def probmatrix():
    m = np.prod(gridsize)
    A = np.zeros((m,m))
    for j in range(m):
        home_state = idx_to_state(j)
        neighbors = [n for n in iter(neighborlist(home_state))]
        for i in [state_to_idx(x) for x in neighbors]:
            A[i,j] = 1/len(neighbors)
    return A

def probat(n):
    m = np.prod(gridsize)
    # Starting probability
    p = np.ones((m,1))/m
    #p[state_to_idx({'re': 5, 'cpu': 500, 'heap': 500})] = 1
    #p[0] = 1
    A = probmatrix()
    for i in range(n):
        p = np.dot(A,p)
    return p

probs = probat(N)


# %%    

for index, row in df.iterrows():
    state = dict(row[varnames])
    state_idx = state_to_idx(state)
    if state_idx in state_visit_counts.keys():
        state_visit_counts[state_idx] += 1
    else:
        state_visit_counts[state_idx] = 1

# %%
# number of all possible states
expected_state_count = np.prod(gridsize)
observed_state_count = len(state_visit_counts.keys())
if expected_state_count != observed_state_count:
    print("The number of observed states (%d) does not match to expected (%d)"
        %(observed_state_count, expected_state_count))

# %% Checking if state visit frequency matches to the expected
# Calculate the all possible connections
tot_connections = np.prod(gridsize)
for variable, values in variables.items():
    tot_connections += 2 * (len(values)-1) * np.prod([len(variables[k]) for k in variables.keys() if k != variable])

visit_frequency_avg_rel_err = 0
observed_state_visit_count = np.zeros((np.prod(gridsize)))
expected_state_visit_count = np.zeros((np.prod(gridsize)))
for state_idx, visit_count in state_visit_counts.items():
    state = idx_to_state(state_idx)
    # interior states has 2n+1 connections (maximum)
    conn_count = 2 * n + 1 
    for var, val in state.items():
        # If there is no more state in the left/right
        # decrease the connections
        if val == min(variables[var]):
            conn_count -= 1
        if val == max(variables[var]):
            conn_count -= 1
    expected_state_visit_prob_infinity = conn_count / tot_connections
    expected_state_visit_count[state_idx]  = int(probs[state_idx,0] * N)
    if abs(expected_state_visit_prob_infinity-probs[state_idx,0])/expected_state_visit_prob_infinity > 0.00001:
        print("Matrix exponential and theory don't match")


    observed_state_visit_count[state_idx] = state_visit_counts[state_idx] 
    rel_error = abs(expected_state_visit_count[state_idx]-observed_state_visit_count[state_idx]) \
                 /expected_state_visit_count[state_idx]
    visit_frequency_avg_rel_err += rel_error

plt.figure(figsize=(8,8))
plt.plot(observed_state_visit_count,'o',label='Observed')
plt.plot(expected_state_visit_count,'.',label='expected')
plt.legend()
plt.xlabel('states orderd by state index')
plt.ylabel('state visit counts')
plt.title('Total number of state visits:%d' % N)
plt.show()
visit_frequency_avg_rel_err /= len(state_visit_counts)
# Close to zero means that the states are visited enough times
# to make meaningful statistics
# If it is large i.e. > .20, better collect more data
print('Average Relative Error in state visit frequency %f'%visit_frequency_avg_rel_err)

# %%
for metric in ['inc_tps','out_tps','cost']:
    missing_indices = df[(np.isnan(df[metric])) | (df[metric] == 0)].index
    missing_values = []
    for missing_index in missing_indices:
        missing_state = dict(df.loc[missing_index][varnames])
        missing_state_idx = state_to_idx(missing_state)
        hist_df_idx = (df.index < missing_index) & \
                      (np.isnan(df[metric]) == False) & \
                      (df[metric] != 0)
        search_idx = hist_df_idx & filter_data_state_idx(missing_state_idx)
        if len(df[search_idx][metric]) == 0:
            try_count = 0
            while len(df[search_idx][metric]) == 0:
                if try_count > 7:
                    print("giving up...")
                    break
                # try, index+1,index-1,index+2,index-2,...
                try_state_idx = missing_state_idx + (try_count//2 + 1) * \
                                (1 if try_count % 2 == 0 else -1)
                search_idx = hist_df_idx & filter_data_state_idx(try_state_idx)
                try_count += 1
            if try_count > 7:
                logging.error('There is no historical data for %s' % missing_state)
            else:
                logging.info('Instead of %s,  %s is used' % (missing_state, idx_to_state(try_state_idx)))

        estimation = np.mean(df[search_idx][metric])
        missing_values.append(estimation)
    df.loc[missing_indices,metric] = missing_values
# %%
df['perf'] = df['out_tps'] * df['re']
df['1st'] = 0
df_aggr = df.groupby(varnames).mean() 
# %%
f = dict()
for output in ['perf', 'cost']:
    f[output] = dict()

    # Constant
    f[output]['0th'] = np.mean(df_aggr[output]) 
    f[output]['D'] = np.mean(df_aggr[output] ** 2) - f[output]['0th']**2

    f[output]['1st'] = dict()
    f[output]['Di'] = dict()
    f[output]['Si'] = dict()
    f[output]['2nd'] = dict()
    f[output]['Dij'] = dict()
    f[output]['Sij'] = dict()

    # First order terms (Performance)
    for feature in varnames:
        f[output]['1st'][feature] = df_aggr.groupby(feature).mean()[output].values - f[output]['0th']
        f[output]['Di'][feature] = np.mean(f[output]['1st'][feature]  ** 2)
        f[output]['Si'][feature] = f[output]['Di'][feature] / f[output]['D'] 

    for i in range(N):
        df.loc[i,'1st'] = f[output]['0th'] 
        for feature in varnames:
            df.loc[i,'1st']  += f[output]['1st'][feature][variables[feature].index(df.loc[i,feature])]

    for i in range(0,n):
        varidx = var_idx_map[i]
        feature = varnames[varidx]
        plt.figure()
        plt.rcParams['text.usetex'] = True
        plt.plot(variables[feature], f[output]['1st'][feature],'*-',label='f1 (%s)'%feature)
        plt.title(r'$f_%d(x_%d)$ (\textbf{%s})'%(i+1,i+1,output))
        plt.xlabel(r'$x_%d$ (\textbf{%s})'%(i+1,feature) )
        plt.ylabel(r'$f_%d$ (\textbf{pps})'%(i+1))
        plt.grid(True)
        ax = plt.gca()
        ax.set_xticks(variables[feature])
        plt.savefig('hdmr_1st_n9_%s_%s.pdf' % (output, feature))
        plt.show()

    # 2nd order HDMR terms
    vmin = math.inf 
    vmax = -math.inf
    for i in range(0,n-1):
        varidx1 = var_idx_map[i]
        feature1 = varnames[varidx1]
        n1 = len(variables[feature1])
        for j in range(i+1,n):
            varidx2 = var_idx_map[j]
            feature2 = varnames[varidx2]
            n2 = len(variables[feature2])
            fij = np.reshape(df_aggr.groupby([feature1,feature2]).mean()[output].values,(n1,n2)) \
                    - np.reshape(f[output]['1st'][feature1],(n1,1)) \
                    - np.reshape(f[output]['1st'][feature2],(1,n2)) \
                    - f[output]['0th']
            f[output]['2nd'][feature1+','+feature2] = fij 
            f[output]['Dij'][feature1+','+feature2] = np.mean(fij**2) 
            f[output]['Sij'][feature1+','+feature2] = f[output]['Dij'][feature1+','+feature2] / f[output]['D'] 
            if fij.min() < vmin: 
                vmin = fij.min()
            if fij.max() > vmax:
                vmax = fij.max()

    # 3rd order HDMR terms
    xvalues3d, yvalues3d, zvalues3d = [] , [] , []
    for i in range(0,n-1):
        varidx1 = var_idx_map[i]
        feature1 = varnames[varidx1]
        n1 = len(variables[feature1])
        for j in range(i+1,n):
            varidx2 = var_idx_map[j]
            feature2 = varnames[varidx2]
            n2 = len(variables[feature2])
            for k in range(j+1,n):
                varidx3 = var_idx_map[k]
                feature3 = varnames[varidx3]
                n3 = len(variables[feature3])
                xvalues3d.append(feature1)
                yvalues3d.append(feature2)
                zvalues3d.append(feature3)
                
                fijk = np.zeros((n1,n2,n3))
                for i1 in range(n1):
                    val1 = variables[feature1][i1]
                    for i2 in range(n2):
                        val2 = variables[feature2][i2]
                        for i3 in range(n3):
                            val3 = variables[feature2][i3]
                            state = {feature1: val1, feature2: val2, feature3: val3}
                            fijk[i1,i2,i3] = df[filter_data(state)][output].mean()

                fijk = fijk \
                      - f[output]['0th'] \
                      - np.reshape(f[output]['1st'][feature1],(n1,1,1)) \
                      - np.reshape(f[output]['1st'][feature2],(1,n2,1)) \
                      - np.reshape(f[output]['1st'][feature3],(1,1,n3)) \
                      - np.reshape(f[output]['2nd'][feature1+','+feature2],(n1,n2,1)) \
                      - np.reshape(f[output]['2nd'][feature1+','+feature3],(n1,1,n3)) \
                      - np.reshape(f[output]['2nd'][feature2+','+feature3],(1,n2,n3))  
                f[output]['3rd'] = fijk

    for i in range(0,n-1):
        varidx1 = var_idx_map[i]
        feature1 = varnames[varidx1]
        n1 = len(variables[feature1])
        for j in range(i+1,n):
            plt.figure(figsize=(10,10))
            plt.rcParams['text.usetex'] = True
            varidx2 = var_idx_map[j]
            feature2 = varnames[varidx2]
            n2 = len(variables[feature2])
            ax = plt.gca()
            fij = f[output]['2nd'][feature1+','+feature2]
            img = ax.imshow(fij,vmin=vmin,vmax=vmax,cmap='GnBu')
            plt.ylabel(r'$x_%d$ (\textbf{%s})'%(i+1,feature1))
            plt.xlabel(r'$x_%d$ (\textbf{%s})'%(j+1,feature2))
            ax.set_xticks(np.arange(n2))
            ax.set_yticks(np.arange(n1))
            ax.set_xticklabels(list(map(str, variables[feature2])))
            ax.set_yticklabels(list(map(str, variables[feature1])))
            for k in range(n1):
                for l in range(n2):
                    ax.text(l,k,'%.4f'%fij[k,l],ha='center',va='center')
            plt.colorbar(img, ax=ax,fraction=0.046, pad=0.04)
            plt.title(r'$f_{%d%d}(x_%d,x_%d)$'%(i+1,j+1,i+1,j+1))
            plt.tight_layout()
            plt.savefig('hdmr_2nd_n9_%s_%d_%d.pdf'%(output,i+1,j+1))
            plt.show()

    colors = cm.get_cmap('tab10', 3)
    fig = plt.figure(figsize=(5,4))
    ax = fig.add_axes([0,0,1,1])
    # Hide the top and right spines of the axis
    ax.spines['right'].set_visible(True)
    ax.spines['top'].set_visible(True)
    ax.xaxis.set_tick_params(which='major', size=10, width=2, direction='in', top='on')
    ax.yaxis.set_tick_params(which='major', size=10, width=2, direction='in', right='on')
    #plt.rcParams['text.usetex'] = True
    for i in range(0,n):
        varidx = var_idx_map[i]
        feature = varnames[varidx]        
        ax.plot(np.arange(1,10),f[output]['1st'][feature],'-',
            color=colors(i),linewidth=3,label=r'$f_%d(x)$'%(i+1))
    ax.set_xticks(np.arange(1,10))
    ax.set_xlim(1,9)
    ax.legend(loc='upper left',frameon=False)
    #plt.title(r'\textbf{Birli YBMG Terimleri (%s)}'%names[output])
    plt.grid(False)
    ax.set_xlabel('x (sıra sayısı)',labelpad=1)
    ax.set_ylabel('%s'% units[output],labelpad=1)
    plt.xticks(**avenirfont)
    plt.yticks(**avenirfont)
    plt.rcParams['pdf.fonttype'] = 42
    #plt.tight_layout()
    plt.savefig('hdmr_1st_n9_%s.pdf' % (output), transparent=False, bbox_inches='tight', dpi=300)
    #plt.show()
    plt.show()

