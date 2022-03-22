# Global Sensitivity Analysis in Cloud Applications
This repository contains the codes necessary to 
produce the data and analysis results presented in
the following manuscript to be published (in Turkish):

**Manuscript**: Hüseyin Kaya, Bulut Uygulamalarında Evrensel Hassasiyet Analizi, TBV Bilgisayar Bilimleri ve Mühendisliği Dergisi (*Hüseyin Kaya, Global Sensitivity Analysis in Cloud Applications, TBV Journal of Computer Science and Engineering*)

# The quickest explanation for the impatient
The purpose of this repository is to show the readers all the details required
to obtain the two figures (*src/hdmr/hdmr_1st_n9_perf.pdf* and *src/hdmr/hdmr_1st_n9_perf.pdf*) 
appearing in my publication above.

If you want to know the details, please proceed.

# Requirements

## For data generation
- An up-and-running Kubernetes cluster. 
  
  It will be too long and unnecessary to include here all the steps necessary to create a Kubernetes cluster. Please consult your System Administrator or if you are on your own and have enough computing resources, you can start reading [Kubernetes Setup Docs](https://kubernetes.io/docs/setup/) **Important** For data generation, a minimal Kubernetes installation like Minikube on your laptop or desktop is not a good idea, because it is impossible to see the possible effect of scaling on single or double core system: the data will be useless in that case. Since the maximum number of replicas in our test scenario  is 9, it is adviced to have at least 9 cores available for the benchmark application that we'll install to Kubernetes. 

  In this project, I've installed OpenShift 4.7 which is a Kubernetes derivative backed
  by RedHat. In the past, I had an experience in installing a vanilla Kubernetes.
  If I compare the two, I should say that, OpenShift is way more stable than vanilla 
  Kubernetes and maintainance is easier. Interestingly, vanilla Kubernetes was easier
  to install but of course hard to maintain. For this project, I think both is ok as long as other requirements are satisfied.

- [Prometheus](https://prometheus.io/)
  
  In order to collect and query the metrics required for the data analysis, Prometheus should 
  be installed to Kubernetes.  

- [Operator-SDK](https://sdk.operatorframework.io/)
  
  The logic to drive the data generation is implemented as a Kubernetes Operator. 
  There are two alternative paths: kubebuilder or Operator-SDK.
  Since I've used OpenShift 4.7 (a Kubernetes derivative), 
  and Openshift promotes the second alternative, 
  I followed  [OpenShift Documentation](https://docs.openshift.com/container-platform/4.7/operators/operator_sdk/golang/osdk-golang-tutorial.html) to create the operator.

- [Golang](https://go.dev/)
  
  The Kubernetes operator can be developed in different ways. In this project, I've selected
  a go-based approach since I want to exploit the full capabilities of a programming language.
   
- [TeaStore](https://github.com/DescartesResearch/TeaStore) application.

  This is the benchmark application that we'll use to create the data in the manuscript. 
  For installation to Kubernetes, you can follow its documentation on GitHub. It is pretty straightforward. 

- [HTTP-Load-Generator](https://github.com/joakimkistowski/HTTP-Load-Generator)

  This utility is to generate a constant load on TeaStore UI. 


## Data analysis
- Python 3 
- Python packages: matplotlib, numpy, pandas


# Creating data
This sole purpose of this section is show the exact steps of how I produced the
data in the manuscript. Because of random nature of the data creating logic, you 
will probably end up in a different data but more or less the data analysis will be same.
If you are more interested in how I used the data to make some analysis, please move on to
Processing data section.

## Starting TeaStore Application
Make sure that TeaStore application is up and running before moving on the next steps.
Please note the endpoint of the TeaStore UI because the load generator will need it soon
to send requests to the correct address.

## Load generation
In HTTP-Load-Generator terminology, there are two concepts to separate the actual 
load generation from the specifiying load profiles. I may use the terminology different 
but the idea is simple enough. In one server which I call it load generator, you create a 
process and waits for further commands from the director. In the director, there is another
Java process to take responsibility of drawing the characteristics of the load i.e. the duration, number of requests in time, and user experience profile.

### Bringing the load generator up
The first step of data generation is create a constant load. 
At the load generator node, the one actually sends the HTTP request, you should bring 
the service up:
     
    java -jar httploadgenerator.jar loadgenerator

- By the way, the *httploadgenerator.jar* comes from [HTTP-Load-Generator](https://github.com/joakimkistowski/HTTP-Load-Generator) installation. 
- If the duration of the load is longer than few minutes, then I suggest to run
  the command in the background via *nohup*.

### Starting the director
Now, the loadgenerator is up and running, we are ready to go. But not too fast.
We still need two files: load profile and user profile.

- **Load Profile** 
  This CSV file specifies the number of requests per second. Its structure is very simple.
  It has two columns separated by a comma. First column is the timestamp, the second
  is the number of requests per second. Here is the first 10 and last 10 lines 
  of constantLoad_100tps.csv. To save some space, I keep the compressed version of it.
  You should uncompress before using.
  
      $ head -10 constantLoad_100tps.csv 
      0.5,100
      1.5,100
      2.5,100
      3.5,100
      4.5,100
      5.5,100
      6.5,100
      7.5,100
      8.5,100
      9.5,100
      $ tail -10 constantLoad_100tps.csv 
      4323750.5,100
      4323751.5,100
      4323752.5,100
      4323753.5,100
      4323754.5,100
      4323755.5,100
      4323756.5,100
      4323757.5,100
      4323758.5,100
      4323759.5,100
  
  As you see, the middle of the 1-second periods are specified in the CSV file.
  This is not my choice. For some reason, HTTP-Load-Generator prefers that way.
  
  Another thing to notice that there are 4323760 lines in the load profile
  meaning that it will take almost 50 days. I want to keep the load-sending 
  process alive as much as possible. Increasing the number of lines is my solution.
  I believe there would be much simpler solutions to get the desired effect
  but I didn't want to spend time. 

  And obviously the second column in always 100 meaning that there will 100 
  requests per second. Actually this is not exactly true. There will be 
  100 GET requests for sure, but some of them will be 302 redirects and the
  redirects are automatically followed. Anyway, it will be slightly larger than
  100 but the important thing is that it will be a constant.
  
- **user profile**
  Now, we need to define the nature of the requests. What will be the contents of 
  those 100 requests? This is the place where HTTP-Load-Generator uses LUA scripts. 
  In this study, I used [teastore_browse.lua](https://github.com/DescartesResearch/TeaStore/blob/master/examples/httploadgenerator/teastore_browse.lua). 
  Make sure that in the LUA script, you change the URL of the teastore to reflect
  your environment. 

Now, at last we are  ready to run the director. Just you issue the following command.

    java -jar httploadgenerator.jar director --ip <load_generator_ip> \
              --load constantLoad_100tps.csv  \
              --lua teastore_browse.lua 

- CSV file describes the load per second whereas LUA script emulates web page visits to
  TeaStore UI. You can use the CSV file under *src/teastore* but make sure to uncompress 
  it before using. You can download [teastore_browse.lua]([teastore_browse.lua](https://github.com/DescartesResearch/TeaStore/blob/master/examples/httploadgenerator/teastore_browse.lua)) but make sure to change the URL of the TeaStore to reflect your environment.
- You should specify the IP of load generator so that the director know where to send the orders.
- If the test duration is long, then I suggest to run the command in the background
  in case your connection drops unexpectedly.

If everyhing goes well, you should see something similar to this:

      $ java -jar httploadgenerator.jar director --ip <load_generator_ip> \
              --load constantLoad_100tps.csv  \
              --lua teastore_browse.lua
      [.... many log lines ...]
      INFO: Created pool of 128 users (LUA contexts, HTTP input generators).
      Target Time = 0.5; Load Intensity = 100.0; #Success = 0; #Failed = 0; #Dropped = 0
      Target Time = 1.5; Load Intensity = 100.0; #Success = 46; #Failed = 0; #Dropped = 0
      Target Time = 2.5; Load Intensity = 100.0; #Success = 63; #Failed = 0; #Dropped = 0
      Target Time = 3.5; Load Intensity = 100.0; #Success = 40; #Failed = 0; #Dropped = 0
      Target Time = 4.5; Load Intensity = 100.0; #Success = 69; #Failed = 0; #Dropped = 0
      Target Time = 5.5; Load Intensity = 100.0; #Success = 95; #Failed = 0; #Dropped = 0

## Running the operator
Let's summarize. TeaStore is running with default options and it gets a constant load of 100
requests per second. It is time to run our Kubernetes Operator to change the TeaStore 
parameters randomly (more correct terminology would be bounded random walk). There are there
parameters I selected: number of replicas, CPU limit and heaplimit for the JVM used in the 
teastore-webui pods. 

I provide the golang-based operator in the *src/operator* folder. Unfortunately, I don't have time 
to explain the exact steps starting scratch. Most of the files are already generated by Operator-SDK.
The core of the operator beats is the Reconcile function in *src/operator/agent_controller.go* where
I implemented the random walk logic in it. There are couple of important things before you try to run 
the operator.

- Make sure that you have access to your Kubernetes environment because the operator will try
  to register itself to the Kubernetes.
- Make sure that your acccess rights do not drop too early because the operator will need to 
  run days to generate enough data. By default access token you generate via oauth lasts only 24 hours.

To run the operator, please issue the following command on *src/operator* folder. 
     
     $ make generate && make manifests && make run

The command will compile the golang codes and create executables under bin folder then run it. 
It will register the operator to the Kubernetes temporaraly. While the operator is running
the Reconcile function is called periodically. The period, which is called  *cooldown period*,
is specified via defaultResync variable in *main.go*. In this study, I set it to 120 seconds allowing 
enough time the parameter changes to settle down. 

If everything goes well, the operator will generate *data.log* under *controller* folder, a text file containing
all parameters and metric measurements. There are more fields in the log file than I explained in the
manuscript. Here, I will only highlight the fields used in the manuscript.

    $ tail -10 rl.log 
    re heap  cpu  inc_tps  out_tps max_sess cur_sess      cpu      ram     cost  goodness    reward re heap   cpu     actionstate   Qvalue
    3  500  300   47.020   56.974       22        0   0.3730   0.5076   0.0418 -418.2514 -169.4247  3  500  0.40     3-400-300-5 -16.9425
    3  500  400  109.742  114.599       27       38   0.4193   0.5021   0.0461 -423.4929   -5.2415  2  500  0.40     3-500-300-3  -0.5242
    2  500  400 1290.617 1724.200      104      118   0.4100   0.5713   0.0308 -189.6246  233.8683  1  500  0.40     3-500-400-2  23.3868
    1  500  400 1609.567 2062.600      122       71   0.3770   0.7380   0.0150  -79.4418  110.1828  1  400  0.40     2-500-400-2  29.7474

* **re** the number of replicas
* **heap** heaplimit for the JVM used in tomcat of teastore-webui pods
* **cpu** CPU limit for the teastore-webui pods
* **out_tps** The number of packets per seconds, produced by the teastore-webui pods, averaged. Performance is therefore re*out_tps
* **cost** cost of teastore-webui in terms of USD per hour. The reference cloud prices are obtained from 
  [kubecost](https://github.com/kubecost/cost-model)

The rest of the fields are not used in this study.

Run the operator for a while and monitor the progress of *data.log* as well as the output 
produced by HTTP-Load-Generator for possible problems. If, for some reason, the load generator
or operator fails, stop all of the process, make a backup of *data.log*, start again. 
You can append the newly generated *data.log* to the old one. In fact, my final data.log contains 
over 20000 lines which took me nearly a month to generate. I had to restart couple of times, so
the end result is a concatenation of few partial results. 

# Processing data  
Now, I assume that the operator run smoothly and generated *data.log*. In case you faced
a problem but hesitate to ask me or don't have enough time for troubleshooting, I provide my *data.log*
under *hdmr* folder. From now on, you only need *data.log* and the Python script *gsa.py* under *hdmr*
with few dependencies. All you have to do is run the Python script and it will run a global sensitivity analysis
tool called High Dimensional Model Representation (HDMR) on *data.log*. For details please look at the manuscript. 
It is in Turkish but the formulas/abstract in the manuscript and Google translation will give a rough idea.
In a few sentences, I am applying HDMR to the data in *data.log*. There are three input parameters:
re (first column), heap (second column), cpu (third column). The output is out_tps and cost columns.
Since there are two outputs, the HDMR analysis is repeated two times for each output. 

To produce the figures I used in the manuscript, under the *src/hdmr* folder please run:

    python3 gsa.py 

Of course, you should install the required Python libraries specified in the first few lines 
of *gsa.py*. 

The Python program will produce quite a few figures. I keep them in the repository so that
you can see the results. Among the figures, I put only the two of them into the manuscript:

- *hdmr_1st_n9_perf.pdf*
  This is the plot of all three first-order HDMR terms when performans is used 
  as an output.
- *hdmr_1st_n9_cost.pdf* 
  Plot of all three HDMR terms when cost is used as HDMR output.







   
